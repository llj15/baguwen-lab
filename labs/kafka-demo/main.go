package main

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
)

const (
	labName           = "kafka-demo"
	datasetURL        = "https://data.gharchive.org/2024-01-01-0.json.gz"
	datasetMD5        = "d4bd9ce833f217e95ffb3fd958138827"
	expectedEvents    = 153619
	partitions        = 12
	groupSampleSize   = 1200
	consumeCheckpoint = 5000
)

type rawEvent struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	CreatedAt string `json:"created_at"`
	Actor     struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	} `json:"actor"`
	Repo struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"repo"`
}

type eventRecord struct {
	Index      int    `json:"index"`
	ID         string `json:"id"`
	Type       string `json:"type"`
	CreatedAt  string `json:"created_at"`
	ActorID    int64  `json:"actor_id"`
	ActorLogin string `json:"actor_login"`
	ActorSeq   int    `json:"actor_seq"`
	RepoID     int64  `json:"repo_id"`
	RepoName   string `json:"repo_name"`
	RepoSeq    int    `json:"repo_seq"`
}

type datasetMetrics struct {
	URL              string      `json:"url"`
	MD5              string      `json:"md5"`
	CompressedBytes  int64       `json:"compressed_bytes"`
	EventCount       int         `json:"event_count"`
	DistinctTypes    int         `json:"distinct_event_types"`
	DistinctRepos    int         `json:"distinct_repos"`
	DistinctActors   int         `json:"distinct_actors"`
	FirstCreatedAt   string      `json:"first_created_at"`
	LastCreatedAt    string      `json:"last_created_at"`
	TopEventTypes    []nameCount `json:"top_event_types"`
	TopRepos         []nameCount `json:"top_repos"`
	TopActors        []nameCount `json:"top_actors"`
	RequiredFieldsOK bool        `json:"required_fields_ok"`
}

type nameCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type produceMetrics struct {
	Topic            string  `json:"topic"`
	Records          int     `json:"records"`
	DurationMS       int64   `json:"duration_ms"`
	RecordsPerSecond float64 `json:"records_per_second"`
}

type consumeMetrics struct {
	Topic                  string  `json:"topic"`
	Records                int     `json:"records"`
	DurationMS             int64   `json:"duration_ms"`
	RecordsPerSecond       float64 `json:"records_per_second"`
	PartitionsUsed         int     `json:"partitions_used"`
	PartitionCounts        []int   `json:"partition_counts"`
	MinPartitionCount      int     `json:"min_partition_count"`
	MaxPartitionCount      int     `json:"max_partition_count"`
	MaxMinRatio            float64 `json:"max_min_ratio"`
	TopPartitionShare      float64 `json:"top_partition_share"`
	KeyPartitionViolations int     `json:"key_partition_violations"`
	PerKeyOrderViolations  int     `json:"per_key_order_violations"`
	UniqueKeys             int     `json:"unique_keys"`
	InitialLagRecords      int     `json:"initial_lag_records"`
	CheckpointConsumed     int     `json:"checkpoint_consumed"`
	CheckpointLagRecords   int     `json:"checkpoint_lag_records"`
	FinalLagRecords        int     `json:"final_lag_records"`
}

type groupMetrics struct {
	RequestedConsumers int   `json:"requested_consumers"`
	ActiveConsumers    int   `json:"active_consumers"`
	PartitionCount     int   `json:"partition_count"`
	Records            int   `json:"records"`
	ConsumerCounts     []int `json:"consumer_counts"`
	DurationMS         int64 `json:"duration_ms"`
}

type scenario struct {
	Name    string      `json:"name"`
	Metrics interface{} `json:"metrics"`
}

type experiment struct {
	Name      string     `json:"name"`
	Scenarios []scenario `json:"scenarios"`
}

type results struct {
	SchemaVersion int                    `json:"schema_version"`
	Lab           string                 `json:"lab"`
	GeneratedAt   string                 `json:"generated_at"`
	Dataset       datasetMetrics         `json:"dataset"`
	Kafka         map[string]interface{} `json:"kafka"`
	Experiments   []experiment           `json:"experiments"`
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	if err := run(context.Background()); err != nil {
		log.Fatalf("experiment failed: %v", err)
	}
}

func run(ctx context.Context) error {
	outputDir := getenv("OUTPUT_DIR", "/data")
	brokers := strings.Split(getenv("KAFKA_BROKERS", "kafka:9092"), ",")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}

	archivePath := filepath.Join(os.TempDir(), "gharchive-2024-01-01-0.json.gz")
	log.Printf("downloading/verifying dataset: %s", datasetURL)
	compressedBytes, err := downloadDataset(ctx, datasetURL, archivePath, datasetMD5)
	if err != nil {
		return err
	}

	log.Printf("parsing dataset")
	events, dataset, err := parseDataset(archivePath, compressedBytes)
	if err != nil {
		return err
	}
	if dataset.EventCount != expectedEvents {
		return fmt.Errorf("dataset event count = %d, want %d", dataset.EventCount, expectedEvents)
	}

	log.Printf("waiting for kafka broker %s", brokers[0])
	if err := waitForKafka(ctx, brokers[0], 90*time.Second); err != nil {
		return err
	}

	topics := []string{"gh-events-rr", "gh-events-by-repo", "gh-events-by-actor"}
	for _, topic := range topics {
		if err := createTopic(ctx, brokers[0], topic, partitions); err != nil {
			return err
		}
	}

	log.Printf("producing %d events to %d topics", len(events), len(topics))
	rrProduce, err := produceEvents(ctx, brokers, "gh-events-rr", events, func(eventRecord) []byte { return nil }, &kafka.RoundRobin{})
	if err != nil {
		return err
	}
	repoProduce, err := produceEvents(ctx, brokers, "gh-events-by-repo", events, func(e eventRecord) []byte {
		return []byte(strconv.FormatInt(e.RepoID, 10))
	}, &kafka.Hash{})
	if err != nil {
		return err
	}
	actorProduce, err := produceEvents(ctx, brokers, "gh-events-by-actor", events, func(e eventRecord) []byte {
		return []byte(strconv.FormatInt(e.ActorID, 10))
	}, &kafka.Hash{})
	if err != nil {
		return err
	}

	log.Printf("consuming topics back for verification")
	rrConsume, err := consumeTopic(ctx, brokers, "gh-events-rr", len(events), "none")
	if err != nil {
		return err
	}
	repoConsume, err := consumeTopic(ctx, brokers, "gh-events-by-repo", len(events), "repo")
	if err != nil {
		return err
	}
	actorConsume, err := consumeTopic(ctx, brokers, "gh-events-by-actor", len(events), "actor")
	if err != nil {
		return err
	}

	log.Printf("running consumer group scaling checks")
	groupScenarios := make([]scenario, 0, 5)
	for _, size := range []int{1, 3, 6, 12, 16} {
		topic := fmt.Sprintf("gh-events-consumer-group-%02d", size)
		if err := createTopic(ctx, brokers[0], topic, partitions); err != nil {
			return err
		}
		metrics, err := runConsumerGroupScenario(ctx, brokers, topic, events[:groupSampleSize], size)
		if err != nil {
			return err
		}
		groupScenarios = append(groupScenarios, scenario{
			Name:    fmt.Sprintf("%d_consumers", size),
			Metrics: metrics,
		})
	}

	result := results{
		SchemaVersion: 1,
		Lab:           labName,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Dataset:       dataset,
		Kafka: map[string]interface{}{
			"broker_image": "docker.io/apache/kafka:3.9.1@sha256:4ceccc577f03f51f6af8dbfda55194d0d892f4fa7913ffbded567ce3895622ed",
			"partitions":   partitions,
			"topics":       append(topics, "gh-events-consumer-group-*"),
		},
		Experiments: []experiment{
			{
				Name: "production",
				Scenarios: []scenario{
					{Name: "round_robin", Metrics: rrProduce},
					{Name: "repo_keyed", Metrics: repoProduce},
					{Name: "actor_keyed", Metrics: actorProduce},
				},
			},
			{
				Name: "partitioning_and_ordering",
				Scenarios: []scenario{
					{Name: "round_robin", Metrics: rrConsume},
					{Name: "repo_keyed", Metrics: repoConsume},
					{Name: "actor_keyed", Metrics: actorConsume},
				},
			},
			{
				Name:      "consumer_group_parallelism",
				Scenarios: groupScenarios,
			},
			{
				Name: "consumer_lag_drain",
				Scenarios: []scenario{
					{Name: "repo_keyed_backlog", Metrics: map[string]int{
						"initial_lag_records":    repoConsume.InitialLagRecords,
						"checkpoint_consumed":    repoConsume.CheckpointConsumed,
						"checkpoint_lag_records": repoConsume.CheckpointLagRecords,
						"final_lag_records":      repoConsume.FinalLagRecords,
					}},
				},
			},
		},
	}

	path := filepath.Join(outputDir, "results.json")
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	log.Printf("wrote %s", path)
	return nil
}

func getenv(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func downloadDataset(ctx context.Context, url, path, expectedMD5 string) (int64, error) {
	if info, err := os.Stat(path); err == nil {
		actual, err := fileMD5(path)
		if err != nil {
			return 0, err
		}
		if actual == expectedMD5 {
			return info.Size(), nil
		}
	}

	tmp := path + ".tmp"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("dataset download status: %s", response.Status)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, err
	}
	file, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	hasher := md5.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hasher), response.Body)
	closeErr := file.Close()
	if copyErr != nil {
		return 0, copyErr
	}
	if closeErr != nil {
		return 0, closeErr
	}
	actual := hex.EncodeToString(hasher.Sum(nil))
	if actual != expectedMD5 {
		return 0, fmt.Errorf("dataset md5 = %s, want %s", actual, expectedMD5)
	}
	if err := os.Rename(tmp, path); err != nil {
		return 0, err
	}
	return written, nil
}

func fileMD5(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := md5.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func parseDataset(path string, compressedBytes int64) ([]eventRecord, datasetMetrics, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, datasetMetrics{}, err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return nil, datasetMetrics{}, err
	}
	defer gz.Close()

	scanner := bufio.NewScanner(gz)
	scanner.Buffer(make([]byte, 1024), 16*1024*1024)

	events := make([]eventRecord, 0, expectedEvents)
	types := map[string]int{}
	repos := map[string]int{}
	actors := map[string]int{}
	repoSeq := map[int64]int{}
	actorSeq := map[int64]int{}
	requiredOK := true
	firstCreated := ""
	lastCreated := ""

	for scanner.Scan() {
		var raw rawEvent
		if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
			return nil, datasetMetrics{}, err
		}
		if raw.ID == "" || raw.Type == "" || raw.CreatedAt == "" || raw.Actor.ID == 0 || raw.Repo.ID == 0 {
			requiredOK = false
			continue
		}

		repoSeq[raw.Repo.ID]++
		actorSeq[raw.Actor.ID]++
		repos[raw.Repo.Name]++
		actors[raw.Actor.Login]++
		types[raw.Type]++
		if firstCreated == "" || raw.CreatedAt < firstCreated {
			firstCreated = raw.CreatedAt
		}
		if lastCreated == "" || raw.CreatedAt > lastCreated {
			lastCreated = raw.CreatedAt
		}
		events = append(events, eventRecord{
			Index:      len(events),
			ID:         raw.ID,
			Type:       raw.Type,
			CreatedAt:  raw.CreatedAt,
			ActorID:    raw.Actor.ID,
			ActorLogin: raw.Actor.Login,
			ActorSeq:   actorSeq[raw.Actor.ID],
			RepoID:     raw.Repo.ID,
			RepoName:   raw.Repo.Name,
			RepoSeq:    repoSeq[raw.Repo.ID],
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, datasetMetrics{}, err
	}

	metrics := datasetMetrics{
		URL:              datasetURL,
		MD5:              datasetMD5,
		CompressedBytes:  compressedBytes,
		EventCount:       len(events),
		DistinctTypes:    len(types),
		DistinctRepos:    len(repos),
		DistinctActors:   len(actors),
		FirstCreatedAt:   firstCreated,
		LastCreatedAt:    lastCreated,
		TopEventTypes:    topN(types, 8),
		TopRepos:         topN(repos, 8),
		TopActors:        topN(actors, 8),
		RequiredFieldsOK: requiredOK,
	}
	return events, metrics, nil
}

func topN(values map[string]int, limit int) []nameCount {
	items := make([]nameCount, 0, len(values))
	for name, count := range values {
		if name == "" {
			name = "(empty)"
		}
		items = append(items, nameCount{Name: name, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Name < items[j].Name
		}
		return items[i].Count > items[j].Count
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func waitForKafka(ctx context.Context, broker string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := kafka.DialContext(ctx, "tcp", broker)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("kafka broker did not become ready within %s", timeout)
}

func createTopic(ctx context.Context, broker, topic string, partitionCount int) error {
	conn, err := kafka.DialContext(ctx, "tcp", broker)
	if err != nil {
		return err
	}
	defer conn.Close()
	controller, err := conn.Controller()
	if err != nil {
		return err
	}
	controllerAddr := net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port))
	controllerConn, err := kafka.DialContext(ctx, "tcp", controllerAddr)
	if err != nil {
		return err
	}
	defer controllerConn.Close()
	err = controllerConn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     partitionCount,
		ReplicationFactor: 1,
	})
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "already exists") {
		return err
	}
	return nil
}

func produceEvents(ctx context.Context, brokers []string, topic string, events []eventRecord, keyFn func(eventRecord) []byte, balancer kafka.Balancer) (produceMetrics, error) {
	start := time.Now()
	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers:      brokers,
		Topic:        topic,
		Balancer:     balancer,
		BatchSize:    1000,
		BatchTimeout: 50 * time.Millisecond,
		RequiredAcks: int(kafka.RequireAll),
	})
	defer writer.Close()

	batch := make([]kafka.Message, 0, 1000)
	written := 0
	for _, event := range events {
		value, err := json.Marshal(event)
		if err != nil {
			return produceMetrics{}, err
		}
		batch = append(batch, kafka.Message{
			Key:   keyFn(event),
			Value: value,
			Time:  time.Now().UTC(),
		})
		if len(batch) == cap(batch) {
			if err := writer.WriteMessages(ctx, batch...); err != nil {
				return produceMetrics{}, err
			}
			written += len(batch)
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		if err := writer.WriteMessages(ctx, batch...); err != nil {
			return produceMetrics{}, err
		}
		written += len(batch)
	}
	duration := time.Since(start)
	return produceMetrics{
		Topic:            topic,
		Records:          written,
		DurationMS:       duration.Milliseconds(),
		RecordsPerSecond: float64(written) / duration.Seconds(),
	}, nil
}

func consumeTopic(ctx context.Context, brokers []string, topic string, expected int, keyMode string) (consumeMetrics, error) {
	start := time.Now()
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		Topic:       topic,
		GroupID:     fmt.Sprintf("%s-verify-%d", topic, time.Now().UnixNano()),
		StartOffset: kafka.FirstOffset,
		MinBytes:    1,
		MaxBytes:    10 * 1024 * 1024,
		MaxWait:     500 * time.Millisecond,
	})
	defer reader.Close()

	consumeCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	partitionCounts := make([]int, partitions)
	keyPartitions := map[string]int{}
	keySeq := map[string]int{}
	keyPartitionViolations := 0
	orderViolations := 0
	checkpointConsumed := 0
	checkpointLag := expected

	for count := 0; count < expected; count++ {
		message, err := reader.FetchMessage(consumeCtx)
		if err != nil {
			return consumeMetrics{}, fmt.Errorf("consume %s after %d/%d messages: %w", topic, count, expected, err)
		}
		if message.Partition >= 0 && message.Partition < len(partitionCounts) {
			partitionCounts[message.Partition]++
		}
		var event eventRecord
		if err := json.Unmarshal(message.Value, &event); err != nil {
			return consumeMetrics{}, err
		}
		if keyMode != "none" {
			key, seq, expectedKey := keyForMode(event, keyMode)
			if string(message.Key) != expectedKey {
				keyPartitionViolations++
			}
			if previous, ok := keyPartitions[key]; ok && previous != message.Partition {
				keyPartitionViolations++
			}
			keyPartitions[key] = message.Partition
			if previous, ok := keySeq[key]; ok {
				if seq != previous+1 {
					orderViolations++
				}
			} else if seq != 1 {
				orderViolations++
			}
			keySeq[key] = seq
		}
		if count+1 == consumeCheckpoint {
			checkpointConsumed = count + 1
			checkpointLag = expected - checkpointConsumed
		}
	}
	duration := time.Since(start)
	minCount, maxCount, used := partitionStats(partitionCounts)
	ratio := 0.0
	if minCount > 0 {
		ratio = float64(maxCount) / float64(minCount)
	}
	topShare := 0.0
	if expected > 0 {
		topShare = float64(maxCount) / float64(expected)
	}
	if checkpointConsumed == 0 {
		checkpointConsumed = expected
		checkpointLag = 0
	}
	return consumeMetrics{
		Topic:                  topic,
		Records:                expected,
		DurationMS:             duration.Milliseconds(),
		RecordsPerSecond:       float64(expected) / duration.Seconds(),
		PartitionsUsed:         used,
		PartitionCounts:        partitionCounts,
		MinPartitionCount:      minCount,
		MaxPartitionCount:      maxCount,
		MaxMinRatio:            ratio,
		TopPartitionShare:      topShare,
		KeyPartitionViolations: keyPartitionViolations,
		PerKeyOrderViolations:  orderViolations,
		UniqueKeys:             len(keyPartitions),
		InitialLagRecords:      expected,
		CheckpointConsumed:     checkpointConsumed,
		CheckpointLagRecords:   checkpointLag,
		FinalLagRecords:        0,
	}, nil
}

func keyForMode(event eventRecord, mode string) (key string, seq int, expectedKey string) {
	switch mode {
	case "repo":
		expectedKey = strconv.FormatInt(event.RepoID, 10)
		return expectedKey, event.RepoSeq, expectedKey
	case "actor":
		expectedKey = strconv.FormatInt(event.ActorID, 10)
		return expectedKey, event.ActorSeq, expectedKey
	default:
		return "", 0, ""
	}
}

func partitionStats(counts []int) (minCount int, maxCount int, used int) {
	minCount = 0
	for i, count := range counts {
		if count > 0 {
			used++
		}
		if i == 0 || count < minCount {
			minCount = count
		}
		if count > maxCount {
			maxCount = count
		}
	}
	return minCount, maxCount, used
}

func runConsumerGroupScenario(ctx context.Context, brokers []string, topic string, events []eventRecord, consumers int) (groupMetrics, error) {
	groupID := fmt.Sprintf("%s-group-%d-%d", topic, consumers, time.Now().UnixNano())
	runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	var total int64
	counts := make([]int, consumers)
	var mu sync.Mutex
	var wg sync.WaitGroup

	start := time.Now()
	for i := 0; i < consumers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			reader := kafka.NewReader(kafka.ReaderConfig{
				Brokers:     brokers,
				Topic:       topic,
				GroupID:     groupID,
				StartOffset: kafka.FirstOffset,
				MinBytes:    1,
				MaxBytes:    10 * 1024 * 1024,
				MaxWait:     500 * time.Millisecond,
			})
			defer reader.Close()
			for {
				if atomic.LoadInt64(&total) >= int64(len(events)) {
					return
				}
				message, err := reader.ReadMessage(runCtx)
				if err != nil {
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
						return
					}
					return
				}
				if len(message.Value) == 0 {
					continue
				}
				mu.Lock()
				counts[index]++
				mu.Unlock()
				if atomic.AddInt64(&total, 1) >= int64(len(events)) {
					cancel()
					return
				}
			}
		}(i)
	}

	time.Sleep(5 * time.Second)
	if _, err := produceEvents(runCtx, brokers, topic, events, func(eventRecord) []byte { return nil }, &kafka.RoundRobin{}); err != nil {
		cancel()
		wg.Wait()
		return groupMetrics{}, err
	}
	wg.Wait()

	if atomic.LoadInt64(&total) != int64(len(events)) {
		return groupMetrics{}, fmt.Errorf("consumer group %s read %d/%d records", groupID, total, len(events))
	}

	active := 0
	for _, count := range counts {
		if count > 0 {
			active++
		}
	}
	return groupMetrics{
		RequestedConsumers: consumers,
		ActiveConsumers:    active,
		PartitionCount:     partitions,
		Records:            len(events),
		ConsumerCounts:     counts,
		DurationMS:         time.Since(start).Milliseconds(),
	}, nil
}

package storage

import (
	"math"
	"slices"
	"strings"
	"time"
)

// ThroughputValues contains locally observed request and token throughput.
type ThroughputValues struct {
	RPM         float64 `json:"rpm"`
	InputTPM    float64 `json:"input_tpm"`
	CacheRead   float64 `json:"cache_read_tpm"`
	CacheCreate float64 `json:"cache_create_tpm"`
	OutputTPM   float64 `json:"output_tpm"`
	TotalTPM    float64 `json:"total_tpm"`
}

// ThroughputPoint contains one fixed local-minute throughput bucket.
type ThroughputPoint struct {
	Minute string `json:"minute"`
	ThroughputValues
}

// ThroughputResult contains active-minute and rolling 60-second observations.
type ThroughputResult struct {
	AverageActiveMinute ThroughputValues  `json:"average_active_minute"`
	PeakRolling60s      ThroughputValues  `json:"peak_rolling_60s"`
	P95Rolling60s       ThroughputValues  `json:"p95_rolling_60s"`
	Series              []ThroughputPoint `json:"series"`
}

type throughputRecord struct {
	timestamp   time.Time
	input       int64
	cacheRead   int64
	cacheCreate int64
	output      int64
}

type throughputWindow struct {
	rpm         int64
	input       int64
	cacheRead   int64
	cacheCreate int64
	output      int64
}

func (w throughputWindow) total() int64 {
	return w.input + w.cacheRead + w.cacheCreate + w.output
}

func (w throughputWindow) values() ThroughputValues {
	return ThroughputValues{
		RPM:         float64(w.rpm),
		InputTPM:    float64(w.input),
		CacheRead:   float64(w.cacheRead),
		CacheCreate: float64(w.cacheCreate),
		OutputTPM:   float64(w.output),
		TotalTPM:    float64(w.total()),
	}
}

// GetThroughput derives local observed throughput from one ordered usage scan.
func (d *DB) GetThroughput(from, to time.Time, source, model string, tzOffset int) (*ThroughputResult, error) {
	clauses := []string{"timestamp BETWEEN ? AND ?"}
	args := []interface{}{from, to}
	if source != "" {
		clauses = append(clauses, "source=?")
		args = append(args, source)
	}
	if model != "" {
		clauses = append(clauses, "model=?")
		args = append(args, model)
	}
	rows, err := d.db.Query(`SELECT timestamp, input_tokens, cache_read_input_tokens,
		cache_creation_input_tokens, output_tokens
		FROM usage_records WHERE `+strings.Join(clauses, " AND ")+`
		ORDER BY timestamp, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]throughputRecord, 0)
	for rows.Next() {
		var record throughputRecord
		if err := rows.Scan(
			&record.timestamp, &record.input, &record.cacheRead,
			&record.cacheCreate, &record.output,
		); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := &ThroughputResult{Series: make([]ThroughputPoint, 0)}
	if len(records) == 0 {
		return result, nil
	}
	buildMinuteSeries(result, records, tzOffset)
	buildRollingThroughput(result, records)
	return result, nil
}

func buildMinuteSeries(result *ThroughputResult, records []throughputRecord, tzOffset int) {
	minuteIndex := make(map[string]int)
	for _, record := range records {
		minute := record.timestamp.Add(-time.Duration(tzOffset) * time.Minute).Format("2006-01-02 15:04")
		index, ok := minuteIndex[minute]
		if !ok {
			index = len(result.Series)
			minuteIndex[minute] = index
			result.Series = append(result.Series, ThroughputPoint{Minute: minute})
		}
		point := &result.Series[index].ThroughputValues
		point.RPM++
		point.InputTPM += float64(record.input)
		point.CacheRead += float64(record.cacheRead)
		point.CacheCreate += float64(record.cacheCreate)
		point.OutputTPM += float64(record.output)
		point.TotalTPM += float64(record.input + record.cacheRead + record.cacheCreate + record.output)
	}

	activeMinutes := float64(len(result.Series))
	for _, point := range result.Series {
		result.AverageActiveMinute.RPM += point.RPM / activeMinutes
		result.AverageActiveMinute.InputTPM += point.InputTPM / activeMinutes
		result.AverageActiveMinute.CacheRead += point.CacheRead / activeMinutes
		result.AverageActiveMinute.CacheCreate += point.CacheCreate / activeMinutes
		result.AverageActiveMinute.OutputTPM += point.OutputTPM / activeMinutes
		result.AverageActiveMinute.TotalTPM += point.TotalTPM / activeMinutes
	}
}

func buildRollingThroughput(result *ThroughputResult, records []throughputRecord) {
	var window throughputWindow
	var rpmValues, inputValues, cacheReadValues, cacheCreateValues, outputValues, totalValues []int64
	left := 0
	for right := 0; right < len(records); {
		timestamp := records[right].timestamp
		groupEnd := right
		for groupEnd < len(records) && records[groupEnd].timestamp.Equal(timestamp) {
			addThroughputRecord(&window, records[groupEnd], 1)
			groupEnd++
		}
		cutoff := timestamp.Add(-time.Minute)
		for left < groupEnd && !records[left].timestamp.After(cutoff) {
			addThroughputRecord(&window, records[left], -1)
			left++
		}

		values := window.values()
		result.PeakRolling60s = maxThroughputValues(result.PeakRolling60s, values)
		for sample := right; sample < groupEnd; sample++ {
			rpmValues = append(rpmValues, window.rpm)
			inputValues = append(inputValues, window.input)
			cacheReadValues = append(cacheReadValues, window.cacheRead)
			cacheCreateValues = append(cacheCreateValues, window.cacheCreate)
			outputValues = append(outputValues, window.output)
			totalValues = append(totalValues, window.total())
		}
		right = groupEnd
	}

	result.P95Rolling60s = ThroughputValues{
		RPM:         float64(nearestRankP95(rpmValues)),
		InputTPM:    float64(nearestRankP95(inputValues)),
		CacheRead:   float64(nearestRankP95(cacheReadValues)),
		CacheCreate: float64(nearestRankP95(cacheCreateValues)),
		OutputTPM:   float64(nearestRankP95(outputValues)),
		TotalTPM:    float64(nearestRankP95(totalValues)),
	}
}

func addThroughputRecord(window *throughputWindow, record throughputRecord, direction int64) {
	window.rpm += direction
	window.input += direction * record.input
	window.cacheRead += direction * record.cacheRead
	window.cacheCreate += direction * record.cacheCreate
	window.output += direction * record.output
}

func maxThroughputValues(current, candidate ThroughputValues) ThroughputValues {
	current.RPM = math.Max(current.RPM, candidate.RPM)
	current.InputTPM = math.Max(current.InputTPM, candidate.InputTPM)
	current.CacheRead = math.Max(current.CacheRead, candidate.CacheRead)
	current.CacheCreate = math.Max(current.CacheCreate, candidate.CacheCreate)
	current.OutputTPM = math.Max(current.OutputTPM, candidate.OutputTPM)
	current.TotalTPM = math.Max(current.TotalTPM, candidate.TotalTPM)
	return current
}

func nearestRankP95(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	slices.Sort(values)
	index := int(math.Ceil(0.95*float64(len(values)))) - 1
	return values[index]
}

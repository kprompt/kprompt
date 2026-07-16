package prometheus

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Result is a normalized Prometheus vector, matrix, scalar, or string result.
type Result struct {
	Type     string
	Series   []Series
	Scalar   *Sample
	Text     string
	Warnings []string
}

// Series contains labels and one or more samples.
type Series struct {
	Metric  map[string]string
	Samples []Sample
}

// Sample is one Prometheus timestamp/value pair.
type Sample struct {
	Timestamp float64
	Value     string
}

type apiEnvelope struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data"`
	ErrorType string          `json:"errorType"`
	Error     string          `json:"error"`
	Warnings  []string        `json:"warnings"`
}

type apiData struct {
	ResultType string          `json:"resultType"`
	Result     json.RawMessage `json:"result"`
}

type apiSeries struct {
	Metric map[string]string `json:"metric"`
	Value  json.RawMessage   `json:"value"`
	Values []json.RawMessage `json:"values"`
}

func decodeResult(raw json.RawMessage) (Result, error) {
	var data apiData
	if err := json.Unmarshal(raw, &data); err != nil {
		return Result{}, fmt.Errorf("decode Prometheus data: %w", err)
	}
	result := Result{Type: data.ResultType}
	switch data.ResultType {
	case "vector":
		var series []apiSeries
		if err := json.Unmarshal(data.Result, &series); err != nil {
			return Result{}, fmt.Errorf("decode Prometheus vector: %w", err)
		}
		result.Series = make([]Series, 0, len(series))
		for _, item := range series {
			sample, err := decodeSample(item.Value)
			if err != nil {
				return Result{}, err
			}
			result.Series = append(result.Series, Series{
				Metric:  cloneMetric(item.Metric),
				Samples: []Sample{sample},
			})
		}
	case "matrix":
		var series []apiSeries
		if err := json.Unmarshal(data.Result, &series); err != nil {
			return Result{}, fmt.Errorf("decode Prometheus matrix: %w", err)
		}
		result.Series = make([]Series, 0, len(series))
		for _, item := range series {
			samples := make([]Sample, 0, len(item.Values))
			for _, rawSample := range item.Values {
				sample, err := decodeSample(rawSample)
				if err != nil {
					return Result{}, err
				}
				samples = append(samples, sample)
			}
			result.Series = append(result.Series, Series{
				Metric:  cloneMetric(item.Metric),
				Samples: samples,
			})
		}
	case "scalar":
		sample, err := decodeSample(data.Result)
		if err != nil {
			return Result{}, err
		}
		result.Scalar = &sample
	case "string":
		sample, err := decodeSample(data.Result)
		if err != nil {
			return Result{}, err
		}
		result.Scalar = &sample
		result.Text = sample.Value
	default:
		return Result{}, fmt.Errorf("unsupported Prometheus result type %q", data.ResultType)
	}
	return result, nil
}

func decodeSample(raw json.RawMessage) (Sample, error) {
	var pair []json.RawMessage
	if err := json.Unmarshal(raw, &pair); err != nil {
		return Sample{}, fmt.Errorf("decode Prometheus sample: %w", err)
	}
	if len(pair) != 2 {
		return Sample{}, fmt.Errorf("decode Prometheus sample: expected timestamp/value pair")
	}
	var timestamp json.Number
	if err := json.Unmarshal(pair[0], &timestamp); err != nil {
		return Sample{}, fmt.Errorf("decode Prometheus timestamp: %w", err)
	}
	seconds, err := strconv.ParseFloat(timestamp.String(), 64)
	if err != nil {
		return Sample{}, fmt.Errorf("decode Prometheus timestamp: %w", err)
	}
	var value string
	if err := json.Unmarshal(pair[1], &value); err != nil {
		return Sample{}, fmt.Errorf("decode Prometheus value: %w", err)
	}
	return Sample{Timestamp: seconds, Value: value}, nil
}

func cloneMetric(metric map[string]string) map[string]string {
	if metric == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(metric))
	for key, value := range metric {
		out[key] = value
	}
	return out
}

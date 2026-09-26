package costmodel

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
)

func LoadCSV(
	path string,
) ([]Observation, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf(
			"read CSV header: %w",
			err,
		)
	}

	columns := make(
		map[string]int,
		len(header),
	)

	for index, name := range header {
		columns[name] = index
	}

	required := []string{
		"state_mb",
		"source_gpu",
		"target_gpu",
		"prepare_us",
		"checkpoint_us",
		"reconfigure_us",
		"restore_us",
		"resume_us",
		"total_us",
		"checkpoint_bytes",
		"restore_bytes",
		"bytes_moved",
	}

	for _, name := range required {
		if _, ok := columns[name]; !ok {
			return nil, fmt.Errorf(
				"missing required CSV column %q",
				name,
			)
		}
	}

	var observations []Observation

	for rowNumber := 2; ; rowNumber++ {
		row, err := reader.Read()

		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, fmt.Errorf(
				"read CSV row %d: %w",
				rowNumber,
				err,
			)
		}

		observation, err :=
			parseObservation(
				row,
				columns,
			)

		if err != nil {
			return nil, fmt.Errorf(
				"parse CSV row %d: %w",
				rowNumber,
				err,
			)
		}

		observations = append(
			observations,
			observation,
		)
	}

	return observations, nil
}

func parseObservation(
	row []string,
	columns map[string]int,
) (Observation, error) {
	intValue := func(
		name string,
	) (int, error) {
		value, err := strconv.Atoi(
			row[columns[name]],
		)

		if err != nil {
			return 0, fmt.Errorf(
				"%s: %w",
				name,
				err,
			)
		}

		return value, nil
	}

	int64Value := func(
		name string,
	) (int64, error) {
		value, err := strconv.ParseInt(
			row[columns[name]],
			10,
			64,
		)

		if err != nil {
			return 0, fmt.Errorf(
				"%s: %w",
				name,
				err,
			)
		}

		return value, nil
	}

	var observation Observation
	var err error

	if observation.StateMB, err =
		intValue("state_mb"); err != nil {
		return Observation{}, err
	}

	if observation.SourceGPU, err =
		intValue("source_gpu"); err != nil {
		return Observation{}, err
	}

	if observation.TargetGPU, err =
		intValue("target_gpu"); err != nil {
		return Observation{}, err
	}

	if observation.PrepareUS, err =
		int64Value("prepare_us"); err != nil {
		return Observation{}, err
	}

	if observation.CheckpointUS, err =
		int64Value("checkpoint_us"); err != nil {
		return Observation{}, err
	}

	if observation.ReconfigureUS, err =
		int64Value("reconfigure_us"); err != nil {
		return Observation{}, err
	}

	if observation.RestoreUS, err =
		int64Value("restore_us"); err != nil {
		return Observation{}, err
	}

	if observation.ResumeUS, err =
		int64Value("resume_us"); err != nil {
		return Observation{}, err
	}

	if observation.TotalUS, err =
		int64Value("total_us"); err != nil {
		return Observation{}, err
	}

	if observation.CheckpointBytes, err =
		int64Value("checkpoint_bytes"); err != nil {
		return Observation{}, err
	}

	if observation.RestoreBytes, err =
		int64Value("restore_bytes"); err != nil {
		return Observation{}, err
	}

	if observation.BytesMoved, err =
		int64Value("bytes_moved"); err != nil {
		return Observation{}, err
	}

	return observation, nil
}

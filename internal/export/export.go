package export

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
)

func WriteJSON(data interface{}, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("error creating JSON file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("error encoding JSON: %w", err)
	}

	fmt.Printf("Data exported to JSON file: %s\n", filename)
	return nil
}

type CSVWriter struct {
	file   *os.File
	writer *csv.Writer
}

func NewCSVWriter(filename string) (*CSVWriter, error) {
	file, err := os.Create(filename)
	if err != nil {
		return nil, fmt.Errorf("error creating CSV file: %w", err)
	}

	return &CSVWriter{
		file:   file,
		writer: csv.NewWriter(file),
	}, nil
}

func (w *CSVWriter) WriteHeader(headers []string) error {
	if err := w.writer.Write(headers); err != nil {
		return fmt.Errorf("error writing CSV headers: %w", err)
	}
	return nil
}

func (w *CSVWriter) WriteRecord(record []string) error {
	if err := w.writer.Write(record); err != nil {
		return fmt.Errorf("error writing CSV record: %w", err)
	}
	return nil
}

func (w *CSVWriter) Close() error {
	w.writer.Flush()
	return w.file.Close()
}

func FormatFilename(username, dataType, format string) string {
	return fmt.Sprintf("%s_%s.%s", username, dataType, format)
}

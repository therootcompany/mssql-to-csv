package mapper

import (
	"fmt"
	"os"
	"strings"
)

type errHandler = func(err error) error

// NamePair has a db column name and corresponding csv field name
type NamePair struct {
	DBColumn string
	CSVField string
}

// Parse returns a 2-D array of DB to CSV field names
func Parse(filepath string, errFn errHandler) ([]NamePair, error) {
	// Format of map.txt is something like
	//
	//    CSV Field Name 1:  DatabaseFieldName
	//    Field Name 2:      DatabaseFieldName
	//
	b, err := os.ReadFile(filepath)
	if nil != err {
		return nil, err
	}

	s := strings.TrimSpace(string(b))
	// This code will be run on Windows.
	// We need to handle \r as well as \r\n and \n
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\n\n", "\n")

	kvs := []NamePair{}

	lines := strings.Split(s, "\n")
	for i := range lines {
		line := lines[i]
		fields := strings.Split(line, ":")
		if 2 != len(fields) {
			err := fmt.Errorf(
				"warn: ignoring bad line %d in %q: %s",
				i, filepath, line,
			)
			if err2 := errFn(err); nil != err2 {
				return kvs, err
			}
			continue
		}

		csvName := strings.TrimSpace(fields[0])
		dbName := strings.TrimSpace(fields[1])
		kvs = append(kvs, NamePair{DBColumn: dbName, CSVField: csvName})
	}

	return kvs, nil
}

package mapper

import (
	"fmt"
	"os"
	"testing"
)

func TestParser(t *testing.T) {
	filepath := "../map.txt"
	kvs, err := Parse(filepath, func(err error) error {
		fmt.Fprintf(os.Stderr, "line error:: %v\n", err)
		return nil
	})
	if nil != err {
		fmt.Fprintf(os.Stderr, "couldn't open %q: %v\n", filepath, err)
		t.Error(err)
		return
	}
	for i := range kvs {
		kv := kvs[i]
		fmt.Println(i, kv.DBColumn, kv.CSVField)
	}
}

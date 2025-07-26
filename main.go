//go:generate go run git.rootprojects.org/root/go-gitver/v2

package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/therootcompany/mssql-to-csv/jsonwriter"
	"github.com/therootcompany/mssql-to-csv/mapper"
	"github.com/therootcompany/mssql-to-csv/mssql"
	"github.com/therootcompany/mssql-to-csv/uploader"

	_ "github.com/denisenkom/go-mssqldb"
	"github.com/jmoiron/sqlx"

	"github.com/joho/godotenv"
)

var (
	commit  = "0000000"
	version = "0.0.0-pre0+0000000"
	date    = "0000-00-00T00:00:00+0000"

	timestamp string
	envpath   string
	outpath   string
	asJSON    bool
	tspath    string
	mappath   string
	debug     bool
)

func main() {
	var logpath string
	var commaStr string
	var sqlQuery string

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			fallthrough
		case "-version":
			fallthrough
		case "--version":
			fallthrough
		case "-V":
			fmt.Printf("mssql-to-csv v%s (%s) %s\n", version, commit[:7], date)
			os.Exit(0)
			return
		}
	}

	cmdname := os.Args[0]
	here := filepath.Dir(cmdname)
	defaultMapPath := filepath.Join(here, "map.txt")
	file, err := os.OpenFile(defaultMapPath, os.O_RDONLY, 0)
	_ = file.Close()
	if err != nil {
		defaultMapPath = ""
	}

	flag.BoolVar(&asJSON, "json", false,
		"output rows as JSON arrays",
	)
	flag.StringVar(&outpath, "csv", "",
		"deprecated, see --out",
	)
	flag.StringVar(&outpath, "out", "",
		"full path to csv or json output, or '-' for stdout (default out.csv or out.json)",
	)
	flag.StringVar(&envpath, "env", filepath.Join(here, ".env"),
		"full path to the .env file with settings and MS SQL & S3 credentials",
	)
	flag.StringVar(&mappath, "map", defaultMapPath,
		"full path to the map.txt that maps MS SQL columns to CSV fields",
	)
	flag.StringVar(&commaStr, "comma", ",",
		"replace ',' with tab, or almost any non-newline unicode character",
	)
	flag.StringVar(&logpath, "log", "",
		"full path to the log file (or stdout if none supplied)",
	)
	flag.StringVar(&timestamp, "timestamp", "2006-01-02_15.04.05",
		"format of timestamp suffix for csv output and S3 key, or '' for no timestamp",
	)
	flag.StringVar(&sqlQuery, "query", "",
		"the query to run (falls back to REPORT_QUERY)",
	)
	flag.BoolVar(&debug, "debug", false,
		"enable additional logging",
	)
	_ = flag.Bool("version", false, "show version info")

	useStdout := !term.IsTerminal(int(os.Stdout.Fd()))
	flag.CommandLine.Visit(func(f *flag.Flag) {
		if f.Name == "out" || f.Name == "csv" {
			useStdout = f.Value.String() == "-"
		}
	})
	flag.Parse()

	// Use .env if available
	_ = godotenv.Load(envpath)

	if len(sqlQuery) == 0 {
		sqlQuery = os.Getenv("REPORT_QUERY")
	}

	if len(logpath) > 0 {
		f, err := os.OpenFile(logpath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if nil != err {
			log.Printf("error opening %q for writing", logpath)
		} else {
			log.Printf("log output to %q", logpath)
			log.SetOutput(f)
		}
	}

	// Copy once, right away
	if err := writeRows(useStdout, sqlQuery, outpath, commaStr); nil != err {
		log.Printf("[ERROR]:\n%v\n", err)
		os.Exit(1)
		return
	}

	durstr := strings.TrimSpace(os.Getenv("REPORT_FREQUENCY"))
	duration, err := time.ParseDuration(durstr)
	if len(durstr) > 0 {
		if nil != err {
			log.Printf("[ERROR]:\ncould not parse duration %q: %v\n", durstr, err)
		}
	}
	if duration == 0 {
		os.Exit(0)
		return
	}

	// Copy in loop with sleep
	// (note: this may actually drift over the course of months)
	for {
		time.Sleep(duration)
		if err := writeRows(useStdout, sqlQuery, outpath, commaStr); nil != err {
			log.Printf("[ERROR]:\n%v\n", err)
			continue
		}

		log.Printf("Waiting %s", duration)
	}
}

func writeRows(useStdout bool, sqlQuery, outpath, commaStr string) error {
	out, err := getWriteCloser(useStdout, outpath)
	if err != nil {
		return err
	}
	roww := getRowWriter(out, commaStr)
	err = copyOut(sqlQuery, roww)
	roww.Flush()
	if !useStdout || err != nil {
		_ = out.Close()
	}
	if err == nil {
		log.Printf("[CSV] Wrote %q\n", tspath)

		if len(os.Getenv("AWS_SECRET_ACCESS_KEY")) > 0 {
			if err := uploadToS3(); nil != err {
				log.Printf("could not upload: %v", err)
			}
		}
	}
	return err
}

func getWriteCloser(useStdout bool, outpath string) (io.WriteCloser, error) {
	var out io.WriteCloser = os.Stdout
	if !useStdout {
		if len(outpath) == 0 {
			outpath = "out.csv"
			if asJSON {
				outpath = "out.json"
			}
		}
		tspath = retimestamp(outpath)
		var err error
		out, err = os.OpenFile(tspath, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			return nil, fmt.Errorf("could not open %q: %w", tspath, err)
		}
	}
	return out, nil
}

func getRowWriter(out io.Writer, commaStr string) RowWriter {
	var roww RowWriter
	if asJSON {
		roww = jsonwriter.NewWriter(out)
	} else {
		runes := []rune(commaStr)
		comma := runes[0]
		csvw := csv.NewWriter(out)
		csvw.Comma = comma
		roww = csvw
	}
	return roww
}

func copyOut(sqlQuery string, roww RowWriter) error {

	// TODO: rename reporter.New
	auth := &mssql.Auth{
		Server:   os.Getenv("MSSQL_SERVER"),
		Port:     os.Getenv("MSSQL_PORT"),
		Username: os.Getenv("MSSQL_USERNAME"),
		Password: os.Getenv("MSSQL_PASSWORD"),
		Instance: os.Getenv("MSSQL_INSTANCE"),
		Catalog:  os.Getenv("MSSQL_CATALOG"),
	}
	tableName := os.Getenv("REPORT_TABLE")
	if len(sqlQuery) == 0 {
		if len(tableName) == 0 {
			return fmt.Errorf("you must set one of either REPORT_QUERY or REPORT_TABLE")
		}
		sqlQuery = fmt.Sprintf("SELECT * FROM %s", tableName)
	} else if len(tableName) > 0 {
		return fmt.Errorf("you must set either REPORT_QUERY or REPORT_TABLE, but not both")
	}
	log.Printf("REPORT_QUERY=%s", sqlQuery)

	db, err := auth.NewConnection()
	if nil != err {
		return fmt.Errorf("could not connect: %w", err)
	}

	var mappings []mapper.NamePair = nil
	if len(mappath) > 0 {
		mappings, err = mapper.Parse(mappath, func(err error) error {
			log.Printf("line error: %v\n", err)
			return nil
		})
		if nil != err {
			return fmt.Errorf("could not read: %w", err)
		}
	}

	err = Report(db, sqlQuery, mappings, roww)
	if nil != err {
		return fmt.Errorf("could not report: %w", err)
	}
	return nil
}

type RowWriter interface {
	Write([]string) error
	Flush()
	Error() error
}

func uploadToS3() error {
	awsAuth := uploader.Auth{
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		Region:          os.Getenv("AWS_REGION"),
	}
	bucket := os.Getenv("AWS_BUCKET")

	// whatever.csv => whatever-2021-04-20.csv
	key := os.Getenv("REPORT_S3_KEY")
	if len(key) == 0 {
		key = filepath.Base(outpath)
	}
	key = retimestamp(key)

	u, err := uploader.New(awsAuth)
	if nil != err {
		return fmt.Errorf("could not upload: %w", err)
	}

	csvr, err := os.Open(tspath)
	if nil != err {
		return fmt.Errorf("could not open %q: %v", tspath, err)
	}
	if err := u.Upload(bucket, key, csvr); nil != err {
		return fmt.Errorf("could not upload: %w", err)
	}

	log.Printf("Uploaded to s3://%s/%s\n", bucket, key)
	return nil
}

func retimestamp(key string) string {
	if len(timestamp) == 0 {
		return key
	}

	// whatever.csv

	// .csv
	ext := filepath.Ext(key)
	// whatever
	key = key[:len(key)-len(ext)]

	// whatever_2006-01-02_15.04.05.csv
	key = fmt.Sprintf("%s_%s%s", key, time.Now().Format(timestamp), ext)
	return key
}

// DBColIndex is a type alias for readability
type DBColIndex = int

// CSVFieldIndex is a type alias for readability
type CSVFieldIndex = int

// DBColName is a type alias for readability
type DBColName = string

// CSVFieldName is a type alias for readability
type CSVFieldName = string

// Report generates the CSV from the database
func Report(
	db *sqlx.DB, sqlQuery string, mappings []mapper.NamePair, roww RowWriter,
) error {
	dateFormat := os.Getenv("REPORT_DATE_FORMAT")
	dateEmpty := os.Getenv("REPORT_DATE_EMPTY")

	rows, err := db.Queryx(sqlQuery)
	if err != nil {
		return fmt.Errorf("could not query %q: %w", sqlQuery, err)
	}
	defer func() {
		// don' forget to close the rows on error
		_ = rows.Close()
	}()

	requiredCols := map[DBColName]CSVFieldIndex{}
	var fieldnames []CSVFieldName = nil
	for i := range mappings {
		pair := mappings[i]
		requiredCols[strings.ToLower(pair.DBColumn)] = i
		fieldnames = append(fieldnames, pair.CSVField)
	}

	// maps between database column order and csv field order
	keepers := map[DBColIndex]CSVFieldIndex{}
	allcols, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("could not get column names: %w", err)
	}
	for dbColIndex := range allcols {
		if len(mappings) == 0 {
			fieldname := allcols[dbColIndex]
			fieldnames = append(fieldnames, fieldname)
			keepers[dbColIndex] = dbColIndex
			continue
		}

		fieldname := strings.ToLower(allcols[dbColIndex])
		if csvFieldIndex, exists := requiredCols[fieldname]; exists {
			keepers[dbColIndex] = csvFieldIndex
		}
	}

	// Write Header
	err = roww.Write(fieldnames)
	if err != nil {
		return fmt.Errorf("could not write column names header: %w", err)
	}

	numfields := len(keepers)
	for rows.Next() {
		var row []any
		row, err = rows.SliceScan()
		if nil != err {
			return err
		}

		fields := make([]string, numfields)
		// convert everything to a string, by any means necessary
		for i, j := range row {
			csvFieldIndex, exists := keepers[i]
			if !exists {
				// skip database columns that we don't need
				continue
			}
			switch v := j.(type) {
			case nil:
				fields[csvFieldIndex] = ""
			case time.Time:
				// MS SQL Server uses 1900-01-01 00:00:00 for empty date
				if v.Format("2006-01-02 15:04:05") == "1900-01-01 00:00:00" || v.IsZero() {
					fields[csvFieldIndex] = dateEmpty
				} else {
					fields[csvFieldIndex] = v.Format(dateFormat)
				}
			case string:
				fields[csvFieldIndex] = v
			case fmt.Stringer:
				if nil != v {
					fields[csvFieldIndex] = v.String()
				}
			default:
				fields[csvFieldIndex] = fmt.Sprintf("%v", v)
			}
			// because MSSQL likes to export VARCHAR with the full possible width???
			// (there's probably a better fix for this, but I don't know much about MSSQL)
			fields[csvFieldIndex] = strings.TrimSpace(fields[csvFieldIndex])
		}

		if err = roww.Write(fields); nil != err {
			return err
		}
	}

	return nil
}

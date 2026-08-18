package main

import (
	"cmp"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

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

	name         = "mssql-to-csv"
	licenseYear  = "2021"
	licenseOwner = "The Root Group, LLC & AJ ONeal"
	licenseType  = "MPL-2.0"
)

// MainConfig holds all CLI flag values and runtime state.
type MainConfig struct {
	cmdname   string
	here      string
	envpath   string
	outpath   string
	asJSON    bool
	mappath   string
	commaStr  string
	logpath   string
	timestamp string
	sqlQuery  string
	debug     bool
	tspath    string // set at runtime by getWriteCloser
}

var cfg MainConfig

// peekOption scans raw args for a flag and returns its value (or a default).
// Handles both "--flag value" and "--flag=value" syntax.
func peekOption(args []string, names []string, def string) (string, bool) {
	for i := range len(args) {
		for _, name := range names {
			if args[i] == name && i+1 < len(args) {
				return args[i+1], true
			}
			// Handle --flag=value syntax
			if strings.HasPrefix(args[i], name+"=") {
				return args[i][len(name)+1:], true
			}
		}
	}
	return def, false
}

// isTTYish reports whether f is a terminal device.
func isTTYish(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	m := os.ModeDevice | os.ModeCharDevice
	return fi.Mode()&m == m
}

// printVersion writes version and license information to w.
// When commit is still the default (no ldflags), falls back to
// runtime/debug.ReadBuildInfo() for VCS metadata.
func printVersion(w io.Writer) {
	if commit == "0000000" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				switch s.Key {
				case "vcs.revision":
					commit = s.Value[:7]
				case "vcs.time":
					date = s.Value
				case "vcs.modified":
					if s.Value == "true" {
						date += "+dirty"
					}
				}
			}
		}
	}
	_, _ = fmt.Fprintf(w, "%s v%s (%s) %s\n", name, version, commit[:7], date)
	_, _ = fmt.Fprintf(w, "Copyright (C) %s %s\n", licenseYear, licenseOwner)
	_, _ = fmt.Fprintf(w, "Licensed under %s\n", licenseType)
}

func main() {
	cfg.cmdname = os.Args[0]
	cfg.here = filepath.Dir(cfg.cmdname)

	// 1. Peek for --env-file / -env and load early so env vars can
	//    influence flag defaults. Flags still override env vars.
	defaultEnvPath := filepath.Join(cfg.here, ".env")
	envFile, hasEnvFlag := peekOption(os.Args[1:], []string{"-env-file", "--env-file", "-env", "--env"}, defaultEnvPath)
	if err := godotenv.Load(envFile); err != nil {
		if hasEnvFlag {
			log.Printf("could not load env file %q: %v", envFile, err)
			os.Exit(2)
		}
		// Default .env missing — silently continue
	}

	defaultMapPath := filepath.Join(cfg.here, "map.txt")
	file, err := os.OpenFile(defaultMapPath, os.O_RDONLY, 0)
	_ = file.Close()
	if err != nil {
		defaultMapPath = ""
	}

	// 2. Let env vars override defaults
	cfg.sqlQuery = cmp.Or(cfg.sqlQuery, os.Getenv("REPORT_QUERY"))

	// 3. Flags override everything
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.BoolVar(&cfg.asJSON, "json", false, "output rows as JSON arrays")
	fs.StringVar(&cfg.outpath, "csv", "", "deprecated, see --out")
	fs.StringVar(&cfg.outpath, "out", "", "full path to csv or json output, or '-' for stdout (default out.csv or out.json)")
	_ = fs.String("env-file", ".env", "full path to the .env file (alias for -env)")
	fs.StringVar(&cfg.mappath, "map", defaultMapPath, "full path to the map.txt that maps MS SQL columns to CSV fields")
	fs.StringVar(&cfg.commaStr, "comma", ",", "replace ',' with tab, or almost any non-newline unicode character")
	fs.StringVar(&cfg.logpath, "log", "", "full path to the log file (or stdout if none supplied)")
	fs.StringVar(&cfg.timestamp, "timestamp", "2006-01-02_15.04.05", "format of timestamp suffix for csv output and S3 key, or '' for no timestamp")
	fs.StringVar(&cfg.sqlQuery, "query", cfg.sqlQuery, "the query to run (falls back to REPORT_QUERY)")
	fs.BoolVar(&cfg.debug, "debug", false, "enable additional logging")
	_ = fs.Bool("version", false, "show version info")

	// Document accepted env vars in usage
	fs.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, "USAGE\n  %s [flags]\n\n", name)
		_, _ = fmt.Fprintf(os.Stderr, "FLAGS\n")
		fs.PrintDefaults()
		_, _ = fmt.Fprintf(os.Stderr, "\nENVIRONMENT\n")
		_, _ = fmt.Fprintf(os.Stderr, "  MSSQL_SERVER            SQL Server hostname\n")
		_, _ = fmt.Fprintf(os.Stderr, "  MSSQL_PORT              SQL Server port (default 1433)\n")
		_, _ = fmt.Fprintf(os.Stderr, "  MSSQL_USERNAME          SQL Server username\n")
		_, _ = fmt.Fprintf(os.Stderr, "  MSSQL_PASSWORD          SQL Server password\n")
		_, _ = fmt.Fprintf(os.Stderr, "  MSSQL_INSTANCE          SQL Server instance (for named instances)\n")
		_, _ = fmt.Fprintf(os.Stderr, "  MSSQL_CATALOG           Database name\n")
		_, _ = fmt.Fprintf(os.Stderr, "  MSSQL_PARAMS            Extra connection params (URI-encoded)\n")
		_, _ = fmt.Fprintf(os.Stderr, "  REPORT_QUERY            SQL query to run\n")
		_, _ = fmt.Fprintf(os.Stderr, "  REPORT_TABLE            Table to export (alternative to REPORT_QUERY)\n")
		_, _ = fmt.Fprintf(os.Stderr, "  REPORT_FREQUENCY        Repeat interval (e.g. 5m, 1h; empty for run-once)\n")
		_, _ = fmt.Fprintf(os.Stderr, "  REPORT_DATE_FORMAT      Go date format for datetime columns\n")
		_, _ = fmt.Fprintf(os.Stderr, "  REPORT_DATE_EMPTY       Value for empty/zero dates\n")
		_, _ = fmt.Fprintf(os.Stderr, "  REPORT_NULL_STRING      Value for NULL fields\n")
		_, _ = fmt.Fprintf(os.Stderr, "  REPORT_DECIMAL_AS_FLOAT Parse DECIMAL/NUMERIC as float64 (default: false)\n")
		_, _ = fmt.Fprintf(os.Stderr, "  REPORT_S3_KEY           S3 key prefix for uploads\n")
		_, _ = fmt.Fprintf(os.Stderr, "  AWS_ACCESS_KEY_ID       AWS access key\n")
		_, _ = fmt.Fprintf(os.Stderr, "  AWS_SECRET_ACCESS_KEY   AWS secret key\n")
		_, _ = fmt.Fprintf(os.Stderr, "  AWS_REGION              AWS region\n")
		_, _ = fmt.Fprintf(os.Stderr, "  AWS_BUCKET              S3 bucket name\n")
	}

	// Handle -V/--version and help before fs.Parse
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "-version", "--version", "-V":
			printVersion(os.Stdout)
			os.Exit(0)
		case "help", "-help", "--help":
			printVersion(os.Stdout)
			_, _ = fmt.Fprintln(os.Stdout, "")
			fs.SetOutput(os.Stdout)
			fs.Usage()
			os.Exit(0)
		}
	}

	useStdout := !isTTYish(os.Stdout)
	if err := fs.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(2)
	}
	// fs.Visit only sees flags that were explicitly set, so it must
	// run after fs.Parse. If -out or -csv was given a real path (not
	// "-"), write to that file instead of stdout.
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "out" || f.Name == "csv" {
			useStdout = f.Value.String() == "-"
		}
	})

	cfg.sqlQuery = cmp.Or(cfg.sqlQuery, os.Getenv("REPORT_QUERY"))

	if len(cfg.logpath) > 0 {
		f, err := os.OpenFile(cfg.logpath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if nil != err {
			log.Printf("error opening %q for writing", cfg.logpath)
		} else {
			log.Printf("log output to %q", cfg.logpath)
			log.SetOutput(f)
		}
	}

	if useStdout {
		defer func() {
			_ = os.Stdout.Close()
		}()
	}

	// Copy once, right away
	if err := writeRows(useStdout, cfg.sqlQuery, cfg.outpath, cfg.commaStr); nil != err {
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
		if err := writeRows(useStdout, cfg.sqlQuery, cfg.outpath, cfg.commaStr); nil != err {
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
		log.Printf("[CSV] Wrote %q\n", cfg.tspath)

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
			if cfg.asJSON {
				outpath = "out.json"
			}
		}
		cfg.tspath = retimestamp(outpath)
		var err error
		out, err = os.OpenFile(cfg.tspath, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			return nil, fmt.Errorf("could not open %q: %w", cfg.tspath, err)
		}
	}
	return out, nil
}

func getRowWriter(out io.Writer, commaStr string) RowWriter {
	var roww RowWriter
	if cfg.asJSON {
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
		Params:   os.Getenv("MSSQL_PARAMS"), // key and value MUST already be URI-encoded
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
	if len(cfg.mappath) > 0 {
		mappings, err = mapper.Parse(cfg.mappath, func(err error) error {
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
	key := cmp.Or(os.Getenv("REPORT_S3_KEY"), filepath.Base(cfg.outpath))
	key = retimestamp(key)

	u, err := uploader.New(awsAuth)
	if nil != err {
		return fmt.Errorf("could not upload: %w", err)
	}

	csvr, err := os.Open(cfg.tspath)
	if nil != err {
		return fmt.Errorf("could not open %q: %v", cfg.tspath, err)
	}
	if err := u.Upload(bucket, key, csvr); nil != err {
		return fmt.Errorf("could not upload: %w", err)
	}

	log.Printf("Uploaded to s3://%s/%s\n", bucket, key)
	return nil
}

func retimestamp(key string) string {
	if len(cfg.timestamp) == 0 {
		return key
	}

	// whatever.csv

	// .csv
	ext := filepath.Ext(key)
	// whatever
	key = key[:len(key)-len(ext)]

	// whatever_2006-01-02_15.04.05.csv
	key = fmt.Sprintf("%s_%s%s", key, time.Now().Format(cfg.timestamp), ext)
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
	dateFormat := cmp.Or(os.Getenv("REPORT_DATE_FORMAT"), "2006-01-02T15:04:05.000Z")
	dateEmpty := os.Getenv("REPORT_DATE_EMPTY")
	nullString := os.Getenv("REPORT_NULL_STRING")
	decimalAsFloat := strings.EqualFold(os.Getenv("REPORT_DECIMAL_AS_FLOAT"), "true")

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
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return fmt.Errorf("could not get column types: %w", err)
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
				fields[csvFieldIndex] = nullString
			case time.Time:
				// MS SQL Server uses 1900-01-01 00:00:00 for empty date
				if v.IsZero() || v.Format("2006-01-02 15:04:05") == "1900-01-01 00:00:00" {
					fields[csvFieldIndex] = dateEmpty
				} else {
					fields[csvFieldIndex] = v.Format(dateFormat)
				}
			case string:
				fields[csvFieldIndex] = v
			case []byte:
				// Numeric types (DECIMAL/NUMERIC, MONEY) and UNIQUEIDENTIFIER
				// scan as []byte from go-mssqldb. Convert to string.
				// Actual binary types (VARBINARY, BINARY, IMAGE) stay as-is.
				switch colTypes[i].DatabaseTypeName() {
				// go-mssqldb reports both DECIMAL and NUMERIC columns as "DECIMAL";
				// NUMERIC is just a T-SQL alias for DECIMAL and is never returned
				// by DatabaseTypeName().
				case "DECIMAL":
					if decimalAsFloat {
						if f, err := strconv.ParseFloat(string(v), 64); err == nil {
							fields[csvFieldIndex] = strconv.FormatFloat(f, 'f', -1, 64)
						} else {
							fields[csvFieldIndex] = string(v)
						}
					} else {
						fields[csvFieldIndex] = string(v)
					}
				case "MONEY", "SMALLMONEY":
					// Keep as string — float64 loses precision for large MONEY values.
					fields[csvFieldIndex] = string(v)
				case "UNIQUEIDENTIFIER":
					fields[csvFieldIndex] = formatUUID(v)
				default:
					fields[csvFieldIndex] = fmt.Sprintf("%v", v)
				}
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

// formatUUID converts a 16-byte MSSQL UNIQUEIDENTIFIER to the standard
// UUID text format. MSSQL stores the first three groups (time_low,
// time_mid, time_hi_and_version) in little-endian byte order, and the
// last two groups (clock_seq_and_node) in big-endian. The standard
// UUID string representation requires this mixed-endian layout.
func formatUUID(b []byte) string {
	if len(b) != 16 {
		return fmt.Sprintf("%x", b)
	}
	// b: [time_low(4) time_mid(2) time_hi(2) clock_seq(2) node(6)]
	// UUID: time_low-time_mid-time_hi-clock_seq-node
	// First three groups are little-endian in MSSQL storage.
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[3], b[2], b[1], b[0], // time_low (LE)
		b[5], b[4], // time_mid (LE)
		b[7], b[6], // time_hi (LE)
		b[8], b[9], // clock_seq (BE)
		b[10], b[11], b[12], b[13], b[14], b[15], // node (BE)
	)
}

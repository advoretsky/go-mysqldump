package mysqldump

import (
	"database/sql"
	"errors"
	"os"
	"path"
	"strings"
	"text/template"
	"time"
)

type table struct {
	Name   string
	SQL    string
	Values string
}

type dump struct {
	DumpVersion   string
	ServerVersion string
	Tables        []*table
	CompleteTime  string
}

const version = "0.1.0"

const tmpl = `-- Go SQL Dump {{ .DumpVersion }}
--
-- ------------------------------------------------------
-- Server version	{{ .ServerVersion }}


{{range .Tables}}
--
-- Table structure for table {{ .Name }}
--

DROP TABLE IF EXISTS {{ .Name }};
{{ .SQL }};
{{ if .Values }}
--
-- Dumping data for table {{ .Name }}
--

LOCK TABLES {{ .Name }} WRITE;
INSERT INTO {{ .Name }} VALUES {{ .Values }};
UNLOCK TABLES;
{{end}}{{ end }}
-- Dump completed on {{ .CompleteTime }}
`

// Creates a MYSQL Dump based on the options supplied through the dumper.
func (d *Dumper) Dump() error {
	name := time.Now().Format(d.format)
	p := path.Join(d.dir, name+".sql")

	// Check dump directory
	if e, _ := exists(p); e {
		return errors.New("Dump '" + name + "' already exists.")
	}

	// Create .sql file
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()

	data := dump{
		DumpVersion: version,
		Tables:      make([]*table, 0),
	}

	// Get server version
	if data.ServerVersion, err = getServerVersion(d.db); err != nil {
		return err
	}

	// Get tables
	tables, err := getTables(d.db)
	if err != nil {
		return err
	}

	// Get sql for each table
	for _, name := range tables {
		if t, err := createTable(d.db, name); err == nil {
			data.Tables = append(data.Tables, t)
		} else {
			return err
		}
	}

	// Set complete time
	data.CompleteTime = time.Now().String()

	// Write dump to file
	t, err := template.New("mysqldump").Parse(tmpl)
	if err != nil {
		return err
	}
	if err = t.Execute(f, data); err != nil {
		return err
	}

	return nil
}

func getTables(db *sql.DB) ([]string, error) {
	tables := make([]string, 0)

	// Get table list
	rows, err := db.Query("SHOW TABLES")
	if err != nil {
		return tables, err
	}
	defer rows.Close()

	// Read result
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return tables, err
		}
		tables = append(tables, table)
	}
	return tables, rows.Err()
}

func getServerVersion(db *sql.DB) (string, error) {
	var server_version string
	if err := db.QueryRow("SELECT version()").Scan(&server_version); err != nil {
		return "", err
	}
	return server_version, nil
}

func createTable(db *sql.DB, name string) (*table, error) {
	var err error
	t := &table{Name: name}

	if t.SQL, err = createTableSQL(db, name); err != nil {
		return nil, err
	}

	if t.Values, err = createTableValues(db, name); err != nil {
		return nil, err
	}

	return t, nil
}

func createTableSQL(db *sql.DB, name string) (string, error) {
	// Get table creation SQL
	var table_return string
	var table_sql string
	err := db.QueryRow("SHOW CREATE TABLE "+name).Scan(&table_return, &table_sql)
	if err != nil {
		return "", err
	}
	if table_return != name {
		return "", errors.New("Returned table is not the same as requested table")
	}

	return table_sql, nil
}

func createTableValues(db *sql.DB, name string) (string, error) {
	// Get Data
	rows, err := db.Query("SELECT * FROM " + name)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	// Get columns
	columns, err := rows.Columns()
	if err != nil {
		return "", err
	}
	if len(columns) == 0 {
		return "", errors.New("No columns in table " + name + ".")
	}

	// Read data
	data_text := make([]string, 0)
	for rows.Next() {
		// Init temp data storage
		data := make([]string, len(columns))
		ptrs := make([]interface{}, len(columns))
		for i, _ := range data {
			ptrs[i] = &data[i]
		}

		// Read data
		if err := rows.Scan(ptrs...); err != nil {
			return "", err
		}
		data_text = append(data_text, "('"+strings.Join(data, "','")+"')")
	}

	return strings.Join(data_text, ","), rows.Err()
}

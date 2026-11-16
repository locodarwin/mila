package db

import "context"

// Ref is a column that references another table's column.
type Ref struct{ Table, Column string }

// TableInfo is a table as the copier sees it, for the tests.
type TableInfo struct {
	Name    string
	Columns []string
	Kinds   map[string]string // text, int, bool, time or bytes
	Refs    map[string]Ref
}

// Tables lists the tables in the order the copier writes them.
func (d *DB) Tables(ctx context.Context) ([]TableInfo, error) {
	ts, err := d.tables(ctx, d.DB)
	if err != nil {
		return nil, err
	}
	names := map[kind]string{kText: "text", kInt: "int", kBool: "bool", kTime: "time", kBytes: "bytes"}
	var out []TableInfo
	for _, t := range ts {
		info := TableInfo{Name: t.name, Kinds: map[string]string{}, Refs: map[string]Ref{}}
		for _, c := range t.cols {
			info.Columns = append(info.Columns, c.name)
			info.Kinds[c.name] = names[c.kind]
		}
		for from, to := range t.refs {
			info.Refs[from] = Ref{Table: to, Column: t.to[from]}
		}
		for from, to := range t.self {
			info.Refs[from] = Ref{Table: t.name, Column: to}
		}
		out = append(out, info)
	}
	return out, nil
}

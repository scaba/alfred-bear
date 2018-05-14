package db

import (
	"database/sql"
	. "fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"

	"github.com/drgrib/alfred-bear/comp"
)

//////////////////////////////////////////////
/// query templates
//////////////////////////////////////////////

const tagQuery = `
	SELECT DISTINCT
		t.ZTITLE 
	FROM 
		ZSFNOTE n 
		INNER JOIN Z_5TAGS nt ON n.Z_PK = nt.Z_5NOTES 
		INNER JOIN ZSFNOTETAG t ON nt.Z_10TAGS = t.Z_PK 
	WHERE 
		n.ZARCHIVED=0 
		AND n.ZTRASHED=0 
		AND lower(t.ZTITLE) LIKE lower('%%%v%%')
	ORDER BY 
		t.ZMODIFICATIONDATE DESC 
`

const recentQuery = `
	SELECT DISTINCT
		ZUNIQUEIDENTIFIER, ZTITLE 
	FROM 
		ZSFNOTE 
	WHERE 
		ZARCHIVED=0 
		AND ZTRASHED=0 
	ORDER BY 
		ZMODIFICATIONDATE DESC 
`

const titleByIDQuery = `
	SELECT DISTINCT
		ZTITLE 
	FROM 
		ZSFNOTE 
	WHERE 
		ZARCHIVED=0 
		AND ZTRASHED=0 
		AND ZUNIQUEIDENTIFIER='%v' 
	ORDER BY 
		ZMODIFICATIONDATE DESC 
`

const notesByTitleQuery = `
	SELECT DISTINCT
		ZUNIQUEIDENTIFIER, ZTITLE 
	FROM 
		ZSFNOTE 
	WHERE 
		ZARCHIVED=0 
		AND ZTRASHED=0 
		AND lower(ZTITLE) LIKE lower('%%%v%%')
	ORDER BY 
		ZMODIFICATIONDATE DESC 
`

const notesByTextQuery = `
	SELECT DISTINCT
		ZUNIQUEIDENTIFIER, ZTITLE 
	FROM 
		ZSFNOTE 
	WHERE 
		ZARCHIVED=0 
		AND ZTRASHED=0 
		AND lower(ZTEXT) LIKE lower('%%%v%%')
	ORDER BY 
		ZMODIFICATIONDATE DESC 
`

//////////////////////////////////////////////
/// Note
//////////////////////////////////////////////

type Note struct {
	ID, Title string
}

//////////////////////////////////////////////
/// NoteList
//////////////////////////////////////////////

type NoteList struct {
	set   map[Note]bool
	slice []Note
}

func NewNoteList() NoteList {
	notes := NoteList{
		set:   map[Note]bool{},
		slice: []Note{},
	}
	return notes
}

func (notes NoteList) Contains(n Note) bool {
	_, exists := notes.set[n]
	return exists
}

func (notes *NoteList) AppendNew(other ...Note) {
	for _, n := range other {
		if !notes.Contains(n) {
			notes.slice = append(notes.slice, n)
			notes.set[n] = true
		}
	}
}

func (notes *NoteList) AppendNewFrom(other NoteList) {
	notes.AppendNew(other.slice...)
}

func (notes NoteList) String() string {
	return Sprintf("NoteList%s", notes.slice)
}

func (notes NoteList) GetSlice() []Note {
	return notes.slice
}

func (notes NoteList) Get(i int) Note {
	return notes.slice[i]
}

func (notes NoteList) Len() int {
	return len(notes.slice)
}

//////////////////////////////////////////////
/// BearDB
//////////////////////////////////////////////

type BearDB struct {
	lite  LiteDB
	limit int
}

func NewBearDB() (BearDB, error) {
	path := comp.Expanduser("~/Library/Containers/net.shinyfrog.bear/Data/Documents/Application Data/database.sqlite")
	lite, err := NewLiteDB(path)
	limit := 25
	db := BearDB{lite, limit}
	return db, err
}

func (db BearDB) limitQuery(q string) string {
	return Sprintf("%s LIMIT %v", q, db.limit)
}

func (db BearDB) SearchTags(s string) ([]string, error) {
	q := Sprintf(tagQuery, s)
	q = db.limitQuery(q)
	tags, err := db.lite.QueryStrings(q)
	return tags, err
}

func toNoteSlice(maps []map[string]string) []Note {
	notes := []Note{}
	for _, m := range maps {
		n := Note{
			ID:    m["ZUNIQUEIDENTIFIER"],
			Title: m["ZTITLE"],
		}
		notes = append(notes, n)
	}
	return notes
}

func (db BearDB) QueryNotes(query string) (NoteList, error) {
	q := db.limitQuery(query)
	maps, err := db.lite.QueryStringMaps(q)
	notes := NewNoteList()
	if err != nil {
		return notes, err
	}
	slice := toNoteSlice(maps)
	notes.AppendNew(slice...)
	return notes, err
}

func (db BearDB) GetRecent() (NoteList, error) {
	notes, err := db.QueryNotes(recentQuery)
	return notes, err
}

func (db BearDB) GetTitle(id string) (string, error) {
	q := Sprintf(titleByIDQuery, id)
	q = db.limitQuery(q)
	titles, err := db.lite.QueryStrings(q)
	if err != nil {
		return "", err
	}
	if len(titles) == 0 {
		return "", Errorf(
			"No notes for ID '%v'", id)
	}
	return titles[0], err
}

func (db BearDB) gapQuery(template, fill string) (NoteList, error) {
	q := Sprintf(template, fill)
	notes, err := db.QueryNotes(q)
	if err != nil {
		return notes, err
	}
	split := strings.Split(fill, " ")
	if len(split) > 1 {
		// word gap search
		join := strings.Join(split, "% %")
		q := Sprintf(template, join)
		moreNotes, err := db.QueryNotes(q)
		if err != nil {
			return notes, err
		}
		notes.AppendNewFrom(moreNotes)
	}
	return notes, err
}

func (db BearDB) SearchNotesByTitle(title string) (NoteList, error) {
	notes, err := db.gapQuery(notesByTitleQuery, title)
	return notes, err
}

func (db BearDB) SearchNotesByText(text string) (NoteList, error) {
	notes, err := db.gapQuery(notesByTextQuery, text)
	return notes, err
}

func (db BearDB) SearchNotes(text string) (NoteList, error) {
	titleNotes, err := db.SearchNotesByTitle(text)
	if err != nil {
		return titleNotes, err
	}
	textNotes, err := db.SearchNotesByText(text)
	if err != nil {
		return titleNotes, err
	}
	titleNotes.AppendNewFrom(textNotes)
	return titleNotes, err
}

//////////////////////////////////////////////
/// LiteDB
//////////////////////////////////////////////

type LiteDB struct {
	db *sql.DB
}

func NewLiteDB(path string) (LiteDB, error) {
	db, err := sql.Open("sqlite3", path)
	lite := LiteDB{db}
	return lite, err
}

func (lite LiteDB) Query(q string) ([]map[string]interface{}, error) {
	results := []map[string]interface{}{}
	rows, err := lite.db.Query(q)
	if err != nil {
		return results, err
	}
	defer rows.Close()
	// credit
	// https://kylewbanks.com/blog/query-result-to-map-in-golang
	cols, err := rows.Columns()
	if err != nil {
		return results, err
	}
	for rows.Next() {
		m := make(map[string]interface{})
		columns := make([]interface{}, len(cols))
		columnPointers := make([]interface{}, len(cols))
		for i, _ := range columns {
			columnPointers[i] = &columns[i]
		}
		if err := rows.Scan(columnPointers...); err != nil {
			return results, err
		}
		for i, colName := range cols {
			val := columnPointers[i].(*interface{})
			m[colName] = *val
		}
		results = append(results, m)
	}
	return results, err
}

func B2S(bs []uint8) string {
	// credit
	// https://stackoverflow.com/a/28848879/130427
	b := make([]byte, len(bs))
	for i, v := range bs {
		b[i] = byte(v)
	}
	return string(b)
}

func (lite LiteDB) QueryStringMaps(q string) ([]map[string]string, error) {
	sResults := []map[string]string{}
	iResults, err := lite.Query(q)
	if err != nil {
		return sResults, err
	}
	for _, iMap := range iResults {
		sMap := map[string]string{}
		for k, v := range iMap {
			sMap[k] = B2S(v.([]uint8))
		}
		sResults = append(sResults, sMap)
	}
	return sResults, err
}

func (lite LiteDB) QueryStrings(q string) ([]string, error) {
	sResults := []string{}
	iResults, err := lite.Query(q)
	if err != nil {
		return sResults, err
	}
	for _, iMap := range iResults {
		s := ""
		for _, v := range iMap {
			s = B2S(v.([]uint8))
		}
		sResults = append(sResults, s)
	}
	return sResults, err
}

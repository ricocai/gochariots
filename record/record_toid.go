// Package record provides the functions of log mainteiners in Chariots.
// It will be implemented using FLStore, but it's currently a mocked in-memory log for prototyping.
package record

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
)

type TOIDRecord struct {
	Timestamp int64
	Host      int
	TOId      int
	LId       int
	Tags      map[string]string
	Pre       TOIDCausality
}

type TOIDCausality struct {
	Host int
	TOId int
}

// ToJSON encodes a record into bytes
func TOIDToJSON(r TOIDRecord) ([]byte, error) {
	return json.Marshal(r)
}

// ToJSONArray encodes slice of records into JSON array
func TOIDToJSONArray(records []TOIDRecord) ([]byte, error) {
	return json.Marshal(records)
}

// JSONToRecord decodes bytes into record
func TOIDJSONToRecord(b []byte, r *TOIDRecord) error {
	return json.Unmarshal(b, &r)
}

// JSONToRecordArray decodes json bytes into slice of records
func TOIDJSONToRecordArray(b []byte, records *[]TOIDRecord) error {
	return json.Unmarshal(b, &records)
}

// ToGob encodes a record into gob bytes
func TOIDToGob(r TOIDRecord) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(r)
	return buf.Bytes(), err
}

// GobToRecord decodes gob bytes into record
func TOIDGobToRecord(b []byte, r *TOIDRecord) error {
	dec := gob.NewDecoder(bytes.NewBuffer(b))
	return dec.Decode(&r)
}

// ToGob encodes a record into gob bytes
func TOIDToGobArray(records []TOIDRecord) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(records)
	return buf.Bytes(), err
}

// GobToRecord decodes gob bytes into record
func TOIDGobToRecordArray(b []byte, r *[]TOIDRecord) error {
	dec := gob.NewDecoder(bytes.NewBuffer(b))
	return dec.Decode(&r)
}
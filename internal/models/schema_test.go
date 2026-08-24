package models

import "testing"

func TestSchema_MarshalRoundTrip(t *testing.T) {
	orig := &Schema{
		Columns: []*Column{
			{Id: 0, Name: "id", Type: COLUMN_TYPE_INT},
			{Id: 1, Name: "name", Type: COLUMN_TYPE_STRING},
		},
	}
	buf := make([]byte, orig.Size())
	orig.MarshalBin(&buf)
	var got Schema
	got.UnmarshalBin(&buf)
	if len(got.Columns) != 2 {
		t.Fatalf("columns = %d, want 2", len(got.Columns))
	}
	if got.Columns[0].Name != "id" || got.Columns[1].Name != "name" {
		t.Fatalf("names = %q, %q", got.Columns[0].Name, got.Columns[1].Name)
	}
	if got.Columns[0].Type != COLUMN_TYPE_INT || got.Columns[1].Type != COLUMN_TYPE_STRING {
		t.Fatalf("types = %v, %v", got.Columns[0].Type, got.Columns[1].Type)
	}
}

func TestEntry_ChecksumRoundTrip(t *testing.T) {
	e := Entry{ColId: 3, RowId: 9, Payload: []byte("hello")}
	buf := make([]byte, e.Size())
	e.MarshalBin(&buf)
	if !EntryCRCValid(buf) {
		t.Fatal("checksum should be valid")
	}
	buf[24] ^= 0xff
	if EntryCRCValid(buf) {
		t.Fatal("tampered entry should fail checksum")
	}
}

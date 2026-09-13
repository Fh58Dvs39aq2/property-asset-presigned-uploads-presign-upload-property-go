package main

import "testing"

func TestDecideUpload(t *testing.T) {
	tests := []struct {
		name      string
		input     uploadIntent
		wantKey   string
		wantLimit int64
		wantError bool
	}{
		{
			name:    "maintenance photo",
			input:   uploadIntent{AssetKind: "maintenance_request", PropertyID: "prop-7", RecordID: "req-19", Filename: "leak.jpg", ContentType: "image/jpeg", SizeBytes: 2 << 20},
			wantKey: "maintenance/prop-7/req-19/leak.jpg", wantLimit: 15 << 20,
		},
		{
			name:    "tenant lease",
			input:   uploadIntent{AssetKind: "tenant_document", PropertyID: "prop-7", RecordID: "tenant-4", Filename: "lease.pdf", ContentType: "application/pdf", SizeBytes: 20 << 20},
			wantKey: "tenant-documents/prop-7/tenant-4/lease.pdf", wantLimit: 25 << 20,
		},
		{
			name:    "inspection image",
			input:   uploadIntent{AssetKind: "inspection_reminder", PropertyID: "prop-8", RecordID: "visit-2", Filename: "meter.png", ContentType: "image/png", SizeBytes: 3 << 20},
			wantKey: "inspections/prop-8/visit-2/meter.png", wantLimit: 10 << 20,
		},
		{
			name:      "inspection rejects PDF",
			input:     uploadIntent{AssetKind: "inspection_reminder", PropertyID: "prop-8", RecordID: "visit-2", Filename: "notes.pdf", ContentType: "application/pdf", SizeBytes: 1000},
			wantError: true,
		},
		{
			name:      "tenant document exceeds policy",
			input:     uploadIntent{AssetKind: "tenant_document", PropertyID: "prop-7", RecordID: "tenant-4", Filename: "archive.pdf", ContentType: "application/pdf", SizeBytes: (25 << 20) + 1},
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := decideUpload(test.input)
			if test.wantError {
				if err == nil {
					t.Fatal("expected policy rejection")
				}
				return
			}
			if err != nil {
				t.Fatalf("decideUpload: %v", err)
			}
			if got.ObjectKey != test.wantKey || got.MaxBytes != test.wantLimit {
				t.Fatalf("decision = %#v, want key %q and limit %d", got, test.wantKey, test.wantLimit)
			}
		})
	}
}

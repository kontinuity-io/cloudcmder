package gcp

import "testing"

func TestParseMachineType(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantVCPU  int32
		wantMem   int64
		wantOK    bool
	}{
		{name: "predefined returns false", in: "n2-standard-4"},
		{name: "another predefined", in: "e2-medium"},
		{name: "legacy custom", in: "custom-8-16384", wantVCPU: 8, wantMem: 16384, wantOK: true},
		{name: "family-prefixed custom", in: "e2-custom-4-8192", wantVCPU: 4, wantMem: 8192, wantOK: true},
		{name: "n2-custom", in: "n2-custom-2-4096", wantVCPU: 2, wantMem: 4096, wantOK: true},
		{name: "malformed missing memory", in: "custom-8"},
		{name: "non-numeric vcpu", in: "custom-x-1024"},
		{name: "zero vcpu rejected", in: "custom-0-1024"},
		{name: "empty string", in: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gv, gm, ok := parseMachineType(tc.in)
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if gv != tc.wantVCPU {
				t.Errorf("vcpu = %d, want %d", gv, tc.wantVCPU)
			}
			if gm != tc.wantMem {
				t.Errorf("mem = %d, want %d", gm, tc.wantMem)
			}
		})
	}
}

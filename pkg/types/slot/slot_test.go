/*
Copyright 2023 The RedisOperator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package slot

import (
	"reflect"
	"testing"
)

var (
	slotsA = &Slots{}
	slotsB = &Slots{}
	slotsC = &Slots{}
	slotsD = &Slots{}

	slotsInterDB = &Slots{}
	slotsInterDC = &Slots{}
	allSlots     *Slots
)

func init() {
	if err := slotsA.Set("0-5461", SlotAssigned); err != nil {
		panic(err)
	}
	if err := slotsB.Set("5462-10922", SlotAssigned); err != nil {
		panic(err)
	}
	if err := slotsC.Set("10923-16383", SlotAssigned); err != nil {
		panic(err)
	}
	if err := slotsD.Set("0-5461,5464,10922,16000-16111", SlotAssigned); err != nil {
		panic(err)
	}
	if err := slotsInterDB.Set("5464,10922", SlotAssigned); err != nil {
		panic(err)
	}
	if err := slotsInterDC.Set("16000-16111", SlotAssigned); err != nil {
		panic(err)
	}

	allSlots = slotsA.Union(slotsB, slotsC)
}

func TestSlots_IsFullfilled(t *testing.T) {
	tests := []struct {
		name         string
		s            *Slots
		wantFullfill bool
	}{
		{
			name:         "slotsA",
			s:            slotsA,
			wantFullfill: false,
		},
		{
			name:         "slotsB",
			s:            slotsB,
			wantFullfill: false,
		},
		{
			name:         "slotsC",
			s:            slotsC,
			wantFullfill: false,
		},
		{
			name:         "allSlots",
			s:            allSlots,
			wantFullfill: true,
		},
		{
			name:         "slotsD",
			s:            slotsD,
			wantFullfill: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.IsFullfilled(); got != tt.wantFullfill {
				t.Errorf("Slots.IsFullfilled() = %v, want %v, %v", got, tt.wantFullfill, tt.s.Slots())
			}
		})
	}
}

func TestSlots_Inter(t *testing.T) {
	type args struct {
		n *Slots
	}
	tests := []struct {
		name string
		s    *Slots
		args args
		want *Slots
	}{
		{
			name: "with_inter_d_b",
			s:    slotsD,
			args: args{
				n: slotsB,
			},
			want: slotsInterDB,
		},
		{
			name: "with_inter_d_c",
			s:    slotsD,
			args: args{
				n: slotsC,
			},
			want: slotsInterDC,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Inter(tt.args.n); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Slots.Inter() = %v, want %v", got.Slots(), tt.want.Slots())
			}
		})
	}
}

func TestSlots_Equals(t *testing.T) {
	slotsA := Slots{}
	slotsB := Slots{}
	for i := 0; i < 1000; i++ {
		if i < 500 {
			status := SlotAssigned
			if i%2 == 0 {
				status = SlotMigrating
			}
			slotsA.Set(i, SlotAssignStatus(status))
		} else {
			slotsA.Set(i, SlotImporting)
		}
	}
	slotsB.Set("0-499", SlotAssigned)
	type args struct {
		old *Slots
	}
	tests := []struct {
		name string
		s    *Slots
		args args
		want bool
	}{
		{
			name: "equals",
			s:    &slotsA,
			args: args{old: &slotsB},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Equals(tt.args.old); got != tt.want {
				t.Errorf("Slots.Equals() = %v, want %v %v %v", got, tt.want, tt.s.Slots(), tt.args.old.Slots())
			}
		})
	}
}

func TestSlots_Union(t *testing.T) {
	type args struct {
		slots []*Slots
	}
	tests := []struct {
		name string
		s    *Slots
		args args
		want *Slots
	}{
		{
			name: "all",
			s:    slotsA,
			args: args{slots: []*Slots{slotsB, slotsC}},
			want: allSlots,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Union(tt.args.slots...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Slots.Union() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSlots_String(t *testing.T) {
	slots := Slots{}
	slots.Set(0, SlotAssigned)
	slots.Set("1-100", SlotImporting)
	slots.Set("1000-2000", SlotMigrating)
	slots.Set("5000-10000", SlotAssigned)
	slots.Set("16111,16121,16131", SlotImporting)
	slots.Set("16112,16122,16132", SlotMigrating)
	slots.Set("16113,16123,16153", SlotAssigned)
	slots.Set("5201,5233,5400", SlotUnAssigned)

	tests := []struct {
		name    string
		s       *Slots
		wantRet string
	}{
		{
			name:    "slots",
			s:       &slots,
			wantRet: "0,1000-2000,5000-5200,5202-5232,5234-5399,5401-10000,16112-16113,16122-16123,16132,16153",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotRet := tt.s.String(); !reflect.DeepEqual(gotRet, tt.wantRet) {
				t.Errorf("Slots.Slots() = %v, want %v", gotRet, tt.wantRet)
			}
		})
	}
}

func TestSlots_Status(t *testing.T) {
	slots := Slots{}
	slots.Set(0, SlotAssigned)
	slots.Set(100, SlotImporting)
	slots.Set(1, SlotMigrating)
	slots.Set(10000, SlotMigrating)
	slots.Set(10000, SlotUnAssigned)
	slots.Set(11000, SlotImporting)
	slots.Set(11000, SlotAssigned)
	type args struct {
		i int
	}

	tests := []struct {
		name string
		s    *Slots
		args args
		want SlotAssignStatus
	}{
		{
			name: "assigned",
			s:    &slots,
			args: args{i: 0},
			want: SlotAssigned,
		},
		{
			name: "importing",
			s:    &slots,
			args: args{i: 100},
			want: SlotImporting,
		},
		{
			name: "migrating",
			s:    &slots,
			args: args{i: 1},
			want: SlotMigrating,
		},
		{
			name: "unassigned",
			s:    &slots,
			args: args{i: 10000},
			want: SlotUnAssigned,
		},
		{
			name: "importing=>assigined",
			s:    &slots,
			args: args{i: 11000},
			want: SlotAssigned,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Status(tt.args.i); got != tt.want {
				t.Errorf("Slots.Status() = %v, want %v, %v", got, tt.want, tt.s)
			}
		})
	}
}

func TestSlots_Sub(t *testing.T) {
	var (
		slotsA = &Slots{}
		slotsB = &Slots{}
		slotsC = &Slots{}

		slotsDiff = &Slots{}
	)

	slotsA.Set("0-1000", SlotImporting)
	slotsA.Set("999-2000", SlotAssigned)
	slotsA.Set("2001-3000", SlotMigrating)
	slotsA.Set("5001-6000", SlotUnAssigned)

	slotsB.Set("0-1000", SlotAssigned)
	slotsB.Set("999-2000", SlotMigrating)
	slotsB.Set("2001-3000", SlotAssigned)
	slotsB.Set("5001-6000", SlotAssigned)

	slotsC.Set("0-998", SlotMigrating)
	slotsC.Set("2000", SlotAssigned)
	slotsC.Set("2100", SlotAssigned)
	slotsC.Set("2101", SlotMigrating)
	slotsC.Set("2102", SlotImporting)
	slotsC.Set("2103", SlotUnAssigned)
	slotsC.Set("5001", SlotAssigned)
	slotsC.Set("5002", SlotImporting)
	slotsC.Set("5003", SlotMigrating)

	slotsDiff.Set("999-1999,2001-2099,2102-3000", SlotAssigned)

	type args struct {
		n *Slots
	}
	tests := []struct {
		name string
		s    *Slots
		args args
		want *Slots
	}{
		{
			name: "no sub",
			s:    slotsA,
			args: args{n: slotsB},
			want: &Slots{},
		},
		{
			name: "sub",
			s:    slotsA,
			args: args{n: slotsC},
			want: slotsDiff,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Sub(tt.args.n); !reflect.DeepEqual(got.data, tt.want.data) {
				t.Errorf("Slots.Sub() = %v, want %v", got.Slots(), tt.want.Slots())
			}
		})
	}
}

func TestSlots_Load(t *testing.T) {
	slots := NewSlots()
	// slots.Load([]string{"1-100", "1002-<-67ed2db8d677e59ec4a4cefb06858cf2a1a89fa1", "77->-e7d1eecce10fd6bb5eb35b9f99a514335d9ba9ca"})
	type args struct {
		v interface{}
	}
	tests := []struct {
		name    string
		slots   *Slots
		args    args
		wantErr bool
	}{
		{
			name: "load",
			args: args{
				v: []string{"5000-5100", "[1002-<-67ed2db8d677e59ec4a4cefb06858cf2a1a89fa1]", "[77->-e7d1eecce10fd6bb5eb35b9f99a514335d9ba9ca]"},
			},
			wantErr: false,
		},
		{
			name: "load more",
			args: args{
				v: []string{"5000-5100,10000-16666"},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := slots.Load(tt.args.v); (err != nil) != tt.wantErr {
				t.Errorf("Slots.Load() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSlots_Count(t *testing.T) {
	slots := NewSlots()
	slots.Load([]string{"1-100", "[1002-<-67ed2db8d677e59ec4a4cefb06858cf2a1a89fa1]", "77->-e7d1eecce10fd6bb5eb35b9f99a514335d9ba9ca]"})

	type args struct {
		status SlotAssignStatus
	}
	tests := []struct {
		name  string
		slots *Slots
		args  args
		wantC int
	}{
		{
			name:  "assigned",
			slots: slots,
			args: args{
				status: SlotAssigned,
			},
			wantC: 100,
		},
		{
			name:  "importing",
			slots: slots,
			args: args{
				status: SlotImporting,
			},
			wantC: 1,
		},
		{
			name:  "migrating",
			slots: slots,
			args: args{
				status: SlotMigrating,
			},
			wantC: 1,
		},
		{
			name:  "unassigned",
			slots: slots,
			args: args{
				status: SlotUnAssigned,
			},
			wantC: 16284,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotC := tt.slots.Count(tt.args.status); gotC != tt.wantC {
				t.Errorf("Slots.Count() = %v, want %v. %v", gotC, tt.wantC, tt.slots.Slots())
			}
		})
	}
}

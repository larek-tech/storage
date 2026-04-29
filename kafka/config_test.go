package kafka

import (
	"testing"
)

func TestNewCfgFromDSN(t *testing.T) {
	cases := []struct {
		name    string
		dsn     string
		want    Cfg
		wantErr bool
	}{
		{
			name: "minimal",
			dsn:  "kafka://b1:9092",
			want: Cfg{Brokers: []string{"b1:9092"}},
		},
		{
			name: "multi broker with auth and options",
			dsn:  "kafka://user:pass@b1:9092,b2:9093/?group_id=g&client_id=c&sasl=plain&tls=true",
			want: Cfg{
				Brokers:  []string{"b1:9092", "b2:9093"},
				User:     "user",
				Password: "pass",
				GroupID:  "g",
				ClientID: "c",
				SASL:     "plain",
				UseTLS:   true,
			},
		},
		{name: "wrong scheme", dsn: "redis://b1:9092", wantErr: true},
		{name: "no brokers", dsn: "kafka://", wantErr: true},
		{name: "broker without port", dsn: "kafka://b1", wantErr: true},
		{name: "bad tls", dsn: "kafka://b1:9092/?tls=maybe", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewCfgFromDSN(tc.dsn)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if !equalCfg(got, tc.want) {
				t.Fatalf("got=%+v want=%+v", got, tc.want)
			}
		})
	}
}

func TestCfgDSNRoundTrip(t *testing.T) {
	in := Cfg{
		Brokers:  []string{"b1:9092", "b2:9093"},
		User:     "u",
		Password: "p",
		GroupID:  "g",
		ClientID: "c",
		SASL:     "scram-sha-256",
		UseTLS:   true,
	}
	out, err := NewCfgFromDSN(in.DSN())
	if err != nil {
		t.Fatal(err)
	}
	if !equalCfg(in, out) {
		t.Fatalf("round-trip mismatch:\nin =%+v\nout=%+v", in, out)
	}
}

func equalCfg(a, b Cfg) bool {
	if a.User != b.User || a.Password != b.Password || a.GroupID != b.GroupID ||
		a.ClientID != b.ClientID || a.SASL != b.SASL || a.UseTLS != b.UseTLS {
		return false
	}
	if len(a.Brokers) != len(b.Brokers) {
		return false
	}
	for i := range a.Brokers {
		if a.Brokers[i] != b.Brokers[i] {
			return false
		}
	}
	return true
}

func TestJSONCodecRoundTrip(t *testing.T) {
	type Payload struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	codec := JSONCodec[Payload]{}
	in := Payload{ID: 7, Name: "egor"}
	b, err := codec.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Payload
	if err := codec.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("got %+v want %+v", out, in)
	}
}

func TestInMemoryRetryPolicy(t *testing.T) {
	p := InMemoryRetryPolicy{MaxAttempts: 3, BaseDelay: 1, MaxDelay: 10}
	if d, retry := p.Next(1, nil); !retry || d != 1 {
		t.Fatalf("attempt 1: d=%v retry=%v", d, retry)
	}
	if d, retry := p.Next(2, nil); !retry || d != 2 {
		t.Fatalf("attempt 2: d=%v retry=%v", d, retry)
	}
	if _, retry := p.Next(3, nil); retry {
		t.Fatalf("attempt 3 should stop")
	}
}

func TestInMemoryDLQ(t *testing.T) {
	d := &InMemoryDLQ{}
	_ = d.Send(RawMessage{Topic: "t", Offset: 1}, nil)
	_ = d.Send(RawMessage{Topic: "t", Offset: 2}, nil)
	got := d.Messages()
	if len(got) != 2 || got[0].Message.Offset != 1 || got[1].Message.Offset != 2 {
		t.Fatalf("unexpected: %+v", got)
	}
}

package exactjson

import "testing"

func TestEqual(t *testing.T) {
	t.Parallel()
	cases := []struct {
		left, right string
		equal       bool
	}{
		{`{"a":[1,0.10,-0],"b":true}`, `{"b":true,"a":[1e0,1e-1,0]}`, true},
		{`9007199254740992`, `9007199254740993`, false},
		{`-9007199254740992`, `-9007199254740993`, false},
		{`1.0000000000000000001`, `1`, false},
		{`1e1000000000`, `10e999999999`, true},
		{`1e-1000000000`, `10e-1000000001`, true},
		{`{"x":1}`, `{"y":1}`, false},
		{`[1,2]`, `[2,1]`, false},
		{`null`, `null`, true},
		{`1`, `"1"`, false},
		{`{`, `{`, false},
		{`1 2`, `1 2`, false},
	}
	for _, test := range cases {
		t.Run(test.left+"/"+test.right, func(t *testing.T) {
			t.Parallel()
			got := Equal([]byte(test.left), []byte(test.right))
			if got != test.equal {
				t.Fatalf("Equal = %v; want %v", got, test.equal)
			}
		})
	}
}

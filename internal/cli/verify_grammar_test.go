package cli

import "testing"

func TestChangeExpectGrammarEnforcement(t *testing.T) {
	cases := []struct {
		name         string
		expect       string
		exitCode     int
		output       string
		wantOK       bool
		wantChecks   []changeVerifyExpectCheck
		wantAdvisory []string
	}{
		{
			name:       "absent expect enforces exit zero",
			expect:     "",
			exitCode:   0,
			wantOK:     true,
			wantChecks: []changeVerifyExpectCheck{{Kind: "exit", Value: "0", OK: true}},
		},
		{
			name:       "absent expect fails on nonzero exit",
			expect:     "",
			exitCode:   1,
			wantOK:     false,
			wantChecks: []changeVerifyExpectCheck{{Kind: "exit", Value: "0", OK: false}},
		},
		{
			name:       "exit atom mismatch fails",
			expect:     "exit 2",
			exitCode:   1,
			wantOK:     false,
			wantChecks: []changeVerifyExpectCheck{{Kind: "exit", Value: "2", OK: false}},
		},
		{
			name:       "exit atom matches nonzero code",
			expect:     "exit 2.",
			exitCode:   2,
			wantOK:     true,
			wantChecks: []changeVerifyExpectCheck{{Kind: "exit", Value: "2", OK: true}},
		},
		{
			name:   "contains match passes",
			expect: "contains `all green`",
			output: "suite: all green\n",
			wantOK: true,
			wantChecks: []changeVerifyExpectCheck{
				{Kind: "exit", Value: "0", OK: true},
				{Kind: "contains", Value: "all green", OK: true},
			},
		},
		{
			name:   "contains mismatch fails even at exit zero",
			expect: "contains `nope`.",
			output: "ok\n",
			wantOK: false,
			wantChecks: []changeVerifyExpectCheck{
				{Kind: "exit", Value: "0", OK: true},
				{Kind: "contains", Value: "nope", OK: false},
			},
		},
		{
			name:   "multi atom and",
			expect: "exit 0 and contains `first` and contains `second`",
			output: "first then second\n",
			wantOK: true,
			wantChecks: []changeVerifyExpectCheck{
				{Kind: "exit", Value: "0", OK: true},
				{Kind: "contains", Value: "first", OK: true},
				{Kind: "contains", Value: "second", OK: true},
			},
		},
		{
			name:   "multi atom fails when one contains misses",
			expect: "exit 0 and contains `first` and contains `second`",
			output: "first only\n",
			wantOK: false,
			wantChecks: []changeVerifyExpectCheck{
				{Kind: "exit", Value: "0", OK: true},
				{Kind: "contains", Value: "first", OK: true},
				{Kind: "contains", Value: "second", OK: false},
			},
		},
		{
			name:   "backticked text keeps its own and",
			expect: "contains `a and b`",
			output: "x a and b y\n",
			wantOK: true,
			wantChecks: []changeVerifyExpectCheck{
				{Kind: "exit", Value: "0", OK: true},
				{Kind: "contains", Value: "a and b", OK: true},
			},
		},
		{
			name:         "unenforceable clause is advisory only",
			expect:       "exit 0 and the output reads well",
			exitCode:     0,
			wantOK:       true,
			wantChecks:   []changeVerifyExpectCheck{{Kind: "exit", Value: "0", OK: true}},
			wantAdvisory: []string{"the output reads well"},
		},
		{
			name:         "advisory clause never rescues a failing atom",
			expect:       "exit 0 and looks right",
			exitCode:     1,
			wantOK:       false,
			wantChecks:   []changeVerifyExpectCheck{{Kind: "exit", Value: "0", OK: false}},
			wantAdvisory: []string{"looks right"},
		},
		{
			name:         "retired and/or promise is advisory, not silently enforced",
			expect:       "exit 0 and/or specific output.",
			exitCode:     0,
			wantOK:       true,
			wantChecks:   []changeVerifyExpectCheck{{Kind: "exit", Value: "0", OK: true}},
			wantAdvisory: []string{"exit 0 and/or specific output"},
		},
		{
			name:         "contains without backticks is advisory",
			expect:       "contains ok",
			output:       "ok\n",
			wantOK:       true,
			wantChecks:   []changeVerifyExpectCheck{{Kind: "exit", Value: "0", OK: true}},
			wantAdvisory: []string{"contains ok"},
		},
		{
			name:     "duplicate exit atoms fail loudly",
			expect:   "exit 1 and exit 0",
			exitCode: 0,
			wantOK:   false,
			wantChecks: []changeVerifyExpectCheck{
				{Kind: "exit-conflict", Value: "1 and 0", OK: false},
			},
		},
		{
			name:   "conflict records contains true alongside",
			expect: "exit 1 and contains `sentinel` and exit 0",
			output: "has sentinel here\n",
			wantOK: false,
			wantChecks: []changeVerifyExpectCheck{
				{Kind: "exit-conflict", Value: "1 and 0", OK: false},
				{Kind: "contains", Value: "sentinel", OK: true},
			},
		},
		{
			name:   "conflict records contains false alongside",
			expect: "exit 1 and contains `sentinel` and exit 0",
			output: "no match\n",
			wantOK: false,
			wantChecks: []changeVerifyExpectCheck{
				{Kind: "exit-conflict", Value: "1 and 0", OK: false},
				{Kind: "contains", Value: "sentinel", OK: false},
			},
		},
		{
			name:   "single exit with contains unchanged",
			expect: "exit 0 and contains `ok`",
			output: "ok\n",
			wantOK: true,
			wantChecks: []changeVerifyExpectCheck{
				{Kind: "exit", Value: "0", OK: true},
				{Kind: "contains", Value: "ok", OK: true},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expectation := parseChangeExpectation(tc.expect)
			checks := evaluateChangeExpectation(expectation, tc.exitCode, tc.output)
			if got := changeExpectChecksPass(checks); got != tc.wantOK {
				t.Fatalf("ok = %v, want %v (checks=%#v)", got, tc.wantOK, checks)
			}
			if len(checks) != len(tc.wantChecks) {
				t.Fatalf("checks = %#v, want %#v", checks, tc.wantChecks)
			}
			for i, want := range tc.wantChecks {
				if checks[i] != want {
					t.Fatalf("check[%d] = %#v, want %#v", i, checks[i], want)
				}
			}
			if len(expectation.Advisory) != len(tc.wantAdvisory) {
				t.Fatalf("advisory = %#v, want %#v", expectation.Advisory, tc.wantAdvisory)
			}
			for i, want := range tc.wantAdvisory {
				if expectation.Advisory[i] != want {
					t.Fatalf("advisory[%d] = %q, want %q", i, expectation.Advisory[i], want)
				}
			}
		})
	}
}

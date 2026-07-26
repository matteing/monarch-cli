package command

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteJSONEscapesTerminalControlsWithoutChangingValues(t *testing.T) {
	value := "\x7f\u0085\u202e\u0301a\u0301"
	var output bytes.Buffer
	if err := writeJSON(&output, map[string]string{"value": value}); err != nil {
		t.Fatal(err)
	}
	for _, char := range []rune{'\x7f', '\u0085', '\u202e', '\u0301'} {
		if strings.ContainsRune(output.String(), char) {
			t.Fatalf("JSON retained unsafe terminal character %U: %q", char, output.String())
		}
	}
	var decoded map[string]string
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["value"] != value {
		t.Fatalf("decoded value = %q, want %q", decoded["value"], value)
	}
}

func TestWriteTableKeepsPipedOutputPlain(t *testing.T) {
	var output bytes.Buffer
	app := application{out: &output}
	if err := app.writeTable([]string{"NAME", "BALANCE"}, [][]string{{"Checking\nAccount", "$42.00"}}); err != nil {
		t.Fatalf("write table: %v", err)
	}
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("piped table contains ANSI styling: %q", output.String())
	}
	if strings.Contains(output.String(), "Checking\nAccount") || !strings.Contains(output.String(), "Checking Account") {
		t.Fatalf("table cell was not sanitized: %q", output.String())
	}
	if strings.Contains(output.String(), "Monarch · Accounts") {
		t.Fatalf("piped table unexpectedly contains the interactive title: %q", output.String())
	}
}

func TestTransactionHelpDocumentsInteractivePagingAndGrouping(t *testing.T) {
	app := application{}
	transactions := app.transactionsCommand()
	list, _, err := transactions.Find([]string{"list"})
	if err != nil {
		t.Fatalf("find transaction list command: %v", err)
	}
	for _, phrase := range []string{"left/right", "up/down", "--cursor"} {
		if !strings.Contains(list.Long, phrase) {
			t.Fatalf("transaction help does not contain %q: %s", phrase, list.Long)
		}
	}
	group := list.Flag("group")
	if group == nil || group.DefValue != "month" || !strings.Contains(group.Usage, "month or none") {
		t.Fatalf("transaction group flag is not documented: %+v", group)
	}
}

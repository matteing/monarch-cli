package command

import (
	"strings"
	"testing"
)

func TestLoginCopyStaysFriendlyAndClear(t *testing.T) {
	command := (&application{}).loginCommand()
	if command.Flags().Lookup("method") != nil {
		t.Fatal("login still exposes authentication method selection")
	}
	for _, phrase := range []string{"save a session credential", "not stored anywhere", "only the session token"} {
		if !strings.Contains(command.Short+"\n"+command.Long, phrase) {
			t.Fatalf("login copy does not contain %q", phrase)
		}
	}
}

func TestLoginSuccessDescribesCredentialMinimization(t *testing.T) {
	message := loginSuccessMessage()
	if !strings.Contains(message, "Only the session token was saved") {
		t.Fatalf("login success = %q", message)
	}
}

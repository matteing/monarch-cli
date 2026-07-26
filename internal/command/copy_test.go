package command

import (
	"strings"
	"testing"

	"github.com/matteing/monarch-cli/internal/session"
)

func TestLoginCopyStaysFriendlyAndClear(t *testing.T) {
	command := (&application{}).loginCommand()
	for _, phrase := range []string{"save a session credential", "not stored anywhere", "only the session token"} {
		if !strings.Contains(command.Short+"\n"+command.Long, phrase) {
			t.Fatalf("login copy does not contain %q", phrase)
		}
	}
}

func TestCookieLoginSuccessDescribesCredentialMinimization(t *testing.T) {
	message := loginSuccessMessage(session.ModeCookie)
	if !strings.Contains(message, "Only the required authentication cookies were retained") {
		t.Fatalf("cookie login success = %q", message)
	}
}

package console_setting

import (
	"strings"
	"testing"
)

func TestValidateUptimeKumaGroupsSupportsEmbedAndTimeWindow(t *testing.T) {
	valid := []string{
		`[{"categoryName":"embedded","embedUrl":"https://status.example.com/embed/status?channelId=1","timeWindowHours":1}]`,
		`[{"categoryName":"embedded","embedUrl":"https://status.example.com/embed","timeWindowHours":"720"}]`,
		`[{"categoryName":"kuma","url":"https://status.example.com","slug":"api"}]`,
	}
	for _, value := range valid {
		if err := validateUptimeKumaGroups(value); err != nil {
			t.Fatalf("valid uptime configuration rejected: %v", err)
		}
	}
}

func TestValidateUptimeKumaGroupsRejectsInvalidEmbedAndTimeWindow(t *testing.T) {
	invalid := []string{
		`[{"categoryName":"missing"}]`,
		`[{"categoryName":"bad-window","url":"https://status.example.com","slug":"api","timeWindowHours":721}]`,
		`[{"categoryName":"fraction","url":"https://status.example.com","slug":"api","timeWindowHours":1.5}]`,
		`[{"categoryName":"danger","embedUrl":"https://status.example.com/<iframe"}]`,
		`[{"categoryName":"html","embedUrl":"javascript:alert(1)"}]`,
	}
	for _, value := range invalid {
		if err := validateUptimeKumaGroups(value); err == nil {
			t.Fatalf("invalid uptime configuration accepted: %s", value)
		}
	}
}

func TestValidateUptimeKumaGroupsPreservesUTF16LengthSemantics(t *testing.T) {
	validDescription := strings.Repeat("明", 200)
	if err := validateUptimeKumaGroups(`[{"categoryName":"unicode","url":"https://status.example.com","slug":"api","description":"` + validDescription + `"}]`); err != nil {
		t.Fatalf("200 UTF-16 units should be accepted: %v", err)
	}
	tooLongDescription := strings.Repeat("😀", 101)
	if err := validateUptimeKumaGroups(`[{"categoryName":"unicode","url":"https://status.example.com","slug":"api","description":"` + tooLongDescription + `"}]`); err == nil {
		t.Fatal("202 UTF-16 units should be rejected")
	}
}

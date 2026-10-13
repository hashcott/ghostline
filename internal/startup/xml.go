package startup

import (
	"bytes"
	"encoding/xml"
	"io"
	"os/user"
	"strings"
	"unicode/utf16"

	"github.com/hashcott/ghostline/internal/brand"
)

// Task is a logon-triggered task that runs elevated without a UAC prompt.
type Task struct {
	Name      string
	Exe       string
	Args      string
	UserID    string // DOMAIN\user
	TimeLimit string // ISO 8601 duration; PT0S = unlimited
}

func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// TaskXML renders a Task Scheduler 1.2 definition.
func TaskXML(t Task) string {
	return `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>` + esc(brand.AppName) + `</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>` + esc(t.UserID) + `</UserId>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>` + esc(t.UserID) + `</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <ExecutionTimeLimit>` + esc(t.TimeLimit) + `</ExecutionTimeLimit>
    <Priority>7</Priority>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>` + esc(t.Exe) + `</Command>
      <Arguments>` + esc(t.Args) + `</Arguments>
    </Exec>
  </Actions>
</Task>
`
}

// TaskMatches reports whether x, a definition as schtasks /Query /XML
// prints it, still starts t at logon: same program and arguments, enabled,
// elevated. Anything else (a missing task's error text included) does not.
func TaskMatches(x string, t Task) bool {
	var def struct {
		Triggers struct {
			Logon []struct {
				Enabled string `xml:"Enabled"`
			} `xml:"LogonTrigger"`
		} `xml:"Triggers"`
		RunLevel string `xml:"Principals>Principal>RunLevel"`
		Enabled  string `xml:"Settings>Enabled"`
		Exec     []struct {
			Command   string `xml:"Command"`
			Arguments string `xml:"Arguments"`
		} `xml:"Actions>Exec"`
	}
	dec := xml.NewDecoder(strings.NewReader(x))
	// The header says UTF-16, but the text reached us already decoded.
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	if err := dec.Decode(&def); err != nil {
		return false
	}
	off := func(v string) bool { return strings.EqualFold(strings.TrimSpace(v), "false") }
	logon := false
	for _, l := range def.Triggers.Logon {
		logon = logon || !off(l.Enabled)
	}
	if !logon || off(def.Enabled) || def.RunLevel != "HighestAvailable" || len(def.Exec) != 1 {
		return false
	}
	unquote := func(v string) string { return strings.Trim(strings.TrimSpace(v), `"`) }
	return strings.EqualFold(unquote(def.Exec[0].Command), t.Exe) &&
		strings.TrimSpace(def.Exec[0].Arguments) == t.Args
}

// EncodeUTF16LE encodes s as UTF-16LE with a BOM, the encoding schtasks /XML expects.
func EncodeUTF16LE(s string) []byte {
	u := utf16.Encode([]rune(s))
	out := make([]byte, 2, 2+2*len(u))
	out[0], out[1] = 0xFF, 0xFE
	for _, c := range u {
		out = append(out, byte(c), byte(c>>8))
	}
	return out
}

// CurrentUserID returns DOMAIN\user for the current process.
func CurrentUserID() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

func userIDOrEmpty() string {
	id, _ := CurrentUserID()
	return id
}

// AutostartTask starts Ghostline minimized at logon.
func AutostartTask(exe string) Task {
	return Task{Name: brand.TaskAutostart, Exe: exe, Args: "--autostart", UserID: userIDOrEmpty(), TimeLimit: "PT0S"}
}

// RecoveryTask restores DNS at the next logon after an unclean shutdown.
func RecoveryTask(exe string) Task {
	return Task{Name: brand.TaskRecovery, Exe: exe, Args: "--restore", UserID: userIDOrEmpty(), TimeLimit: "PT5M"}
}

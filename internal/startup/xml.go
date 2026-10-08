package startup

import (
	"bytes"
	"encoding/xml"
	"os/user"
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

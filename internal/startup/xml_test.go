package startup_test

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/startup"
	"github.com/stretchr/testify/require"
)

func TestTaskXML(t *testing.T) {
	x := startup.TaskXML(startup.Task{Name: "Ghostline", Exe: `C:\Program Files\Ghostline\ghostline.exe`,
		Args: "--autostart", UserID: `PC\Đức`, TimeLimit: "PT0S"})
	require.Contains(t, x, "<RunLevel>HighestAvailable</RunLevel>")
	require.Contains(t, x, "<LogonTrigger>")
	require.Contains(t, x, `<Command>C:\Program Files\Ghostline\ghostline.exe</Command>`)
	require.Contains(t, x, "<Arguments>--autostart</Arguments>")
	require.Contains(t, x, `<UserId>PC\Đức</UserId>`)
	require.Contains(t, x, "<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>")
	require.Contains(t, x, "<DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>")
	require.Contains(t, x, "<MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>")
	var v any
	dec := xml.NewDecoder(strings.NewReader(x))
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil } // in-memory string is UTF-8
	require.NoError(t, dec.Decode(&v))
}

func TestTaskXML_EscapesSpecialChars(t *testing.T) {
	x := startup.TaskXML(startup.Task{Name: "x", Exe: `C:\A&B\<g>.exe`, UserID: "u", TimeLimit: "PT0S"})
	require.Contains(t, x, `C:\A&amp;B\&lt;g&gt;.exe`)
}

func TestRecoveryAndAutostartTasks(t *testing.T) {
	r := startup.RecoveryTask(`C:\g.exe`)
	require.Equal(t, brand.TaskRecovery, r.Name)
	require.Equal(t, "--restore", r.Args)
	require.Equal(t, "PT5M", r.TimeLimit)
	a := startup.AutostartTask(`C:\g.exe`)
	require.Equal(t, brand.TaskAutostart, a.Name)
	require.Equal(t, "--autostart", a.Args)
	require.Equal(t, "PT0S", a.TimeLimit)
	require.NotEmpty(t, a.UserID)
}

func TestUTF16WithBOM(t *testing.T) {
	b := startup.EncodeUTF16LE("<a/>")
	require.Equal(t, []byte{0xFF, 0xFE, '<', 0, 'a', 0, '/', 0, '>', 0}, b)
}

func TestGuardTaskXML(t *testing.T) {
	x := startup.GuardTaskXML(startup.Guard{PowerShell: `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`,
		Script: `C:\ProgramData\Ghostline\guard.ps1`, State: `C:\Users\Đức\AppData\Roaming\Ghostline\state.json`,
		UserSID: "S-1-5-21-1-2-3-1001", Log: `C:\ProgramData\Ghostline\guard.log`})
	require.Contains(t, x, "<UserId>S-1-5-18</UserId>") // SYSTEM: no window, survives logoff
	require.Contains(t, x, "<Interval>PT1M</Interval>")
	require.Contains(t, x, "<BootTrigger>")
	require.Contains(t, x, "EventID=1116 or EventID=1117")
	require.Contains(t, x, `-File &#34;C:\ProgramData\Ghostline\guard.ps1&#34;`)
	require.Contains(t, x, `-State &#34;C:\Users\Đức\AppData\Roaming\Ghostline\state.json&#34;`)
	require.Contains(t, x, `-UserSid &#34;S-1-5-21-1-2-3-1001&#34;`)
	require.Contains(t, x, `-TaskName &#34;`+brand.TaskGuard+`&#34;`)
	require.NotContains(t, x, "ghostline.exe") // must outlive a quarantined exe
	var v any
	dec := xml.NewDecoder(strings.NewReader(x))
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	require.NoError(t, dec.Decode(&v))
}

func TestGuardScriptIsASCII(t *testing.T) {
	// Windows PowerShell reads a BOM-less script as ANSI.
	require.NotEmpty(t, startup.GuardScript)
	for i, b := range startup.GuardScript {
		require.Less(t, b, byte(0x80), "non-ASCII byte at %d", i)
	}
}

func TestTaskMatches(t *testing.T) {
	want := startup.AutostartTask(`C:\Program Files\Ghostline\ghostline.exe`)
	ours := startup.TaskXML(want)
	require.True(t, startup.TaskMatches(ours, want))
	// schtasks /Query /XML prints the stored definition; case and quotes
	// around the path do not matter to Windows.
	quoted := strings.Replace(ours, `<Command>C:\Program Files`, `<Command>"c:\program files`, 1)
	quoted = strings.Replace(quoted, `ghostline.exe</Command>`, `ghostline.exe"</Command>`, 1)
	require.True(t, startup.TaskMatches(quoted, want))

	cases := map[string]string{
		"other exe":        strings.Replace(ours, `Ghostline\ghostline.exe`, `Old\ghostline.exe`, 1),
		"no --autostart":   strings.Replace(ours, "<Arguments>--autostart</Arguments>", "<Arguments></Arguments>", 1),
		"task disabled":    strings.Replace(ours, "<AllowStartOnDemand>true</AllowStartOnDemand>\n    <Enabled>true</Enabled>", "<AllowStartOnDemand>true</AllowStartOnDemand>\n    <Enabled>false</Enabled>", 1),
		"trigger disabled": strings.Replace(ours, "<LogonTrigger>\n      <Enabled>true</Enabled>", "<LogonTrigger>\n      <Enabled>false</Enabled>", 1),
		"not elevated":     strings.Replace(ours, "HighestAvailable", "LeastPrivilege", 1),
		"no logon trigger": strings.Replace(ours, "LogonTrigger", "BootTrigger", 2),
		"not xml":          "ERROR: The system cannot find the file specified.",
	}
	for name, x := range cases {
		require.NotEqual(t, ours, x, name)
		require.False(t, startup.TaskMatches(x, want), name)
	}
}

// What schtasks /Query /XML prints for a task TaskXML registered: Windows
// drops default elements (Enabled true) and reorders the rest.
const storedAutostart = `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Ghostline</Description>
    <URI>\Ghostline</URI>
  </RegistrationInfo>
  <Principals>
    <Principal id="Author">
      <UserId>S-1-5-21-3468109081-2647591708-3057436553-1001</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <StartWhenAvailable>true</StartWhenAvailable>
    <IdleSettings>
      <StopOnIdleEnd>true</StopOnIdleEnd>
      <RestartOnIdle>false</RestartOnIdle>
    </IdleSettings>
  </Settings>
  <Triggers>
    <LogonTrigger>
      <UserId>SUPERPC\ducha</UserId>
    </LogonTrigger>
  </Triggers>
  <Actions Context="Author">
    <Exec>
      <Command>C:\Program Files\Ghostline\Ghostline\ghostline.exe</Command>
      <Arguments>--autostart</Arguments>
    </Exec>
  </Actions>
</Task>
`

func TestTaskMatches_StoredDefinition(t *testing.T) {
	require.True(t, startup.TaskMatches(storedAutostart, startup.AutostartTask(`C:\Program Files\Ghostline\Ghostline\ghostline.exe`)))
	require.False(t, startup.TaskMatches(storedAutostart, startup.AutostartTask(`C:\Program Files\Ghostline\ghostline.exe`)))
}

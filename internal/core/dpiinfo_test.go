package core

import (
	"testing"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/hashcott/ghostline/internal/dpi/zapret2"
	"github.com/hashcott/ghostline/internal/dpi/zapret2/strategies"
	"github.com/stretchr/testify/require"
)

func TestDPIInfo_FromEngines(t *testing.T) {
	z := zapret2.New(func() strategies.List { return strategies.List{} })
	got := dpiInfo([]dpi.Engine{z}, dpi.InterceptorInfo{Mechanism: "m", AVExclusions: true})
	require.Equal(t, app.DPIInfo{Engines: []app.DPIEngineInfo{{ID: "zapret2", Exe: z.Exe()}}, Mechanism: "m", AVExclusions: true}, got)
}

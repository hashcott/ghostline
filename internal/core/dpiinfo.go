package core

import (
	"path"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/dpi"
)

// dpiInfo describes this OS's engines and packet capture for the DPI page.
func dpiInfo(engines []dpi.Engine, ic dpi.InterceptorInfo) app.DPIInfo {
	info := app.DPIInfo{Engines: []app.DPIEngineInfo{}, Mechanism: ic.Mechanism, AVExclusions: ic.AVExclusions}
	for _, e := range engines {
		info.Engines = append(info.Engines, app.DPIEngineInfo{ID: e.ID(), Exe: path.Base(e.Exe())})
	}
	return info
}

package extensions

import (
	"context"
	"os/exec"

	mb "github.com/PopcornDev1/mobilebridge/pkg/mobilebridge"
)

var mbListDevices = mb.ListDevices
var mbLookPath = exec.LookPath

type mobilebridgeAdapter struct{}

func (mobilebridgeAdapter) Available() bool {
	_, err := mbLookPath("adb")
	return err == nil
}

func (mobilebridgeAdapter) ListDevices(ctx context.Context) ([]MobileDevice, error) {
	devices, err := mbListDevices(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]MobileDevice, 0, len(devices))
	for _, d := range devices {
		name := d.Model
		if name == "" {
			name = d.Serial
		}
		out = append(out, MobileDevice{
			UDID:     d.Serial,
			Name:     name,
			Platform: "android",
			Model:    d.Model,
		})
	}
	return out, nil
}

func (mobilebridgeAdapter) Connect(ctx context.Context, udid string) (*MobileSession, error) {
	return nil, ErrUnavailable
}

func init() {
	Registry.SetMobile(mobilebridgeAdapter{})
}

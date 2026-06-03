package commands

import "strings"

type TypeSpec struct {
	Type    string
	Label   string
	Target  string
	Summary string
}

var builtinTypes = []TypeSpec{
	{Type: "ping", Label: "ping", Target: TargetDevice, Summary: "device health check"},
	{Type: "reboot", Label: "reboot", Target: TargetDevice, Summary: "restart the device"},
	{Type: "sync_config", Label: "sync_config", Target: TargetDevice, Summary: "refresh the device configuration"},
	{Type: "exit_kiosk", Label: "exit_kiosk", Target: TargetDevice, Summary: "leave kiosk mode"},
	{Type: "launch_companion_app", Label: "launch_companion_app", Target: TargetDevice, Summary: "launch a declared companion app"},
}

func BuiltinTypes() []TypeSpec {
	return append([]TypeSpec(nil), builtinTypes...)
}

func IsBuiltinType(commandType string) bool {
	commandType = strings.TrimSpace(commandType)
	if commandType == "" {
		return false
	}
	for _, spec := range builtinTypes {
		if spec.Type == commandType {
			return true
		}
	}
	return false
}

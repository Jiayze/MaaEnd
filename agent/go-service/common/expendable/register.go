package expendable

import maa "github.com/MaaXYZ/maa-framework-go/v4"

// Register registers expendable custom recognitions.
func Register() {
	maa.AgentServerRegisterCustomRecognition(componentName, &Recognition{})
	maa.AgentServerRegisterCustomRecognition(boxComponentName, &BoxRecognition{})
}

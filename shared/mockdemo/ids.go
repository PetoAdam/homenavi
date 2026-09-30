package mockdemo

import "github.com/google/uuid"

type Binding struct {
	SeedDeviceID uuid.UUID
	HDPDeviceID  string
}

const (
	SofaLampHDPDeviceID       = "mock/sofa-lamp"
	TVBacklightHDPDeviceID    = "mock/tv-backlight"
	KitchenPendantHDPDeviceID = "mock/kitchen-pendant"
	CoffeeMakerHDPDeviceID    = "mock/coffee-maker"
	BedroomBlindHDPDeviceID   = "mock/bedroom-blind"
	DeskLampHDPDeviceID       = "mock/desk-lamp"
	EntrySensorHDPDeviceID    = "mock/entry-sensor"
	AirPurifierHDPDeviceID    = "mock/air-purifier"
)

var bindings = []Binding{
	{SeedDeviceID: uuid.MustParse("0d47ed4e-6bb2-45b5-bbfc-c53ff2377701"), HDPDeviceID: SofaLampHDPDeviceID},
	{SeedDeviceID: uuid.MustParse("4d7a0d13-927f-46c0-88bc-eb5ff4988688"), HDPDeviceID: TVBacklightHDPDeviceID},
	{SeedDeviceID: uuid.MustParse("56b56529-3223-4214-ba31-0f6e7e4d59fa"), HDPDeviceID: KitchenPendantHDPDeviceID},
	{SeedDeviceID: uuid.MustParse("0f03eb64-6301-46ff-8532-96ee39c3e215"), HDPDeviceID: CoffeeMakerHDPDeviceID},
	{SeedDeviceID: uuid.MustParse("b6ffdce7-10b6-4eb7-ac0f-f5ec8110e6f7"), HDPDeviceID: BedroomBlindHDPDeviceID},
	{SeedDeviceID: uuid.MustParse("f837b2f3-f01a-4aef-940b-d2f5cc785f95"), HDPDeviceID: DeskLampHDPDeviceID},
	{SeedDeviceID: uuid.MustParse("0f22caa8-a112-4312-84c0-ec4896e8b6f3"), HDPDeviceID: EntrySensorHDPDeviceID},
	{SeedDeviceID: uuid.MustParse("6ee50240-4763-49ba-b0f9-b0f9e29be6bf"), HDPDeviceID: AirPurifierHDPDeviceID},
}

func Bindings() []Binding {
	out := make([]Binding, len(bindings))
	copy(out, bindings)
	return out
}

func HDPDeviceIDForSeedDevice(id uuid.UUID) (string, bool) {
	for _, binding := range bindings {
		if binding.SeedDeviceID == id {
			return binding.HDPDeviceID, true
		}
	}
	return "", false
}

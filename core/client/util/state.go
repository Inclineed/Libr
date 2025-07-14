package util

import (
	"github.com/devlup-labs/Libr/core/client/types"
)

func GetOnlineMods() ([]types.Mod, error) {
	mods := []types.Mod{
		{
			IP:        "49.36.179.166",
			Port:      "59877",
			PublicKey: "amcF3bQQo4C8ozlbXkM5m0CujJba5ygEc093zCs636k=",
		},
	}
	return mods, nil
}

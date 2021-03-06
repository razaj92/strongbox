package app

import (
	"fmt"
	"os"
	"reflect"

	"github.com/fatih/color"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/mvisonneau/strongbox/config"
)

var version = "<devel>"
var cfg config.Config
var s State
var v Vault

func execute(c *cli.Context) error {
	configureLogging()
	log.Debugf("Function: %v", c.Command.FullName())

	switch c.Command.FullName() {
	case "transit use":
		if c.NArg() != 1 {
			fmt.Println("Usage: strongbox transit use [transit_key_name]")
			os.Exit(1)
		}
		s.Load()
		s.SetVaultTransitKey(c.Args().Get(0))
	case "transit info":
		s.Load()
		v.ConfigureClient()
		v.GetTransitInfo()
	case "transit list":
		v.ConfigureClient()
		v.ListTransitKeys()
	case "transit create":
		if c.NArg() != 1 {
			fmt.Println("Usage: strongbox transit create [transit_key_name]")
			os.Exit(1)
		}
		s.Load()
		v.ConfigureClient()
		v.CreateTransitKey(c.Args().Get(0))
		s.SetVaultTransitKey(c.Args().Get(0))
	case "secret write":
		if c.NArg() != 3 {
			fmt.Println("Usage: strongbox secret write [secret_name] [key] [value]")
			os.Exit(1)
		}
		s.Load()
		v.ConfigureClient()
		s.WriteSecretKey(c.Args().Get(0), c.Args().Get(1), v.Encrypt(c.Args().Get(2)))
	case "secret read":
		if c.NArg() != 2 {
			fmt.Println("Usage: strongbox secret read [secret_name] [key]")
			os.Exit(1)
		}
		s.Load()
		v.ConfigureClient()
		fmt.Println(v.Decrypt(s.ReadSecretKey(c.Args().Get(0), c.Args().Get(1))))
	case "secret delete":
		s.Load()
		switch c.NArg() {
		case 1:
			s.DeleteSecret(c.Args().Get(0))
		case 2:
			s.DeleteSecretKey(c.Args().Get(0), c.Args().Get(1))
		default:
			fmt.Println("Usage: strongbox secret read [secret_name] (key)")
			os.Exit(1)
		}
	case "secret list":
		s.Load()
		switch c.NArg() {
		case 1:
			s.ListSecrets(c.Args().Get(0))
		default:
			s.ListSecrets("")
		}
	case "get-secret-path":
		s.Load()
		fmt.Println(s.VaultSecretPath())
	case "set-secret-path":
		if c.NArg() != 1 {
			fmt.Println("Usage: strongbox set-secret-path [secret_path]")
			os.Exit(1)
		}
		s.Load()
		s.SetVaultSecretPath(c.Args().Get(0))
	case "init":
		s.Init()
	case "status":
		s.Load()
		v.ConfigureClient()
		s.Status()
		v.Status()
	case "plan":
		run("plan")
	case "apply":
		run("apply")
	default:
		log.Fatalf("Function %v not implemented yet", c.Command.FullName())
	}

	return nil
}

func run(action string) {
	s.Load()
	v.ConfigureClient()

	// Fetch local values
	local := make(map[string]map[string]string)
	for k, l := range s.Secrets {
		if local[k] == nil {
			local[k] = make(map[string]string)
		}
		for m, n := range l {
			local[k][m] = v.Decrypt(n)
		}
	}

	// Fetch remote values
	remote := make(map[string]map[string]string)
	d, err := v.Client.Logical().List(s.Vault.SecretPath)
	if err != nil {
		log.Fatalf("Vault error: %v", err)
		os.Exit(1)
	}

	if d != nil {
		for _, k := range d.Data["keys"].([]interface{}) {
			if remote[k.(string)] == nil {
				remote[k.(string)] = make(map[string]string)
			}

			l, err := v.Client.Logical().Read(s.Vault.SecretPath + k.(string))
			if err != nil {
				log.Fatalf("Vault error: %v", err)
				os.Exit(1)
			}

			for m, n := range l.Data {
				remote[k.(string)][m] = n.(string)
			}
		}
	}

	eq := reflect.DeepEqual(local, remote)
	if eq {
		color.Green("Nothing to do! Local state and remote Vault config are in sync.")
	} else {
		compare(local, remote, action)
	}
}

func compare(local map[string]map[string]string, remote map[string]map[string]string, action string) {
	var addSecret, deleteSecret []string
	addSecretKey := make(map[string][]string)
	deleteSecretKey := make(map[string][]string)
	addSecretKeyCount := 0
	deleteSecretKeyCount := 0

	for kl, vl := range local {
		foundSecret := false
		for kr, vr := range remote {
			if kl == kr {
				foundSecret = true
				for klk := range vl {
					foundSecretKey := false
					for krk := range vr {
						if klk == krk {
							foundSecretKey = true
							break
						}
					}

					if !foundSecretKey {
						if addSecretKey[kl] == nil {
							addSecretKey[kl] = make([]string, 0)
						}
						addSecretKey[kl] = append(addSecretKey[kl], klk)
						addSecretKeyCount++
					}
				}
				break
			}
		}

		if !foundSecret {
			addSecret = append(addSecret, kl)
			addSecretKey[kl] = make([]string, 0)
			for klk := range vl {
				addSecretKey[kl] = append(addSecretKey[kl], klk)
				addSecretKeyCount++
			}
		}
	}

	for kr, vr := range remote {
		foundSecret := false
		for kl, vl := range local {
			if kr == kl {
				foundSecret = true
				for krk := range vr {
					foundSecretKey := false
					for klk := range vl {
						if krk == klk {
							foundSecretKey = true
							break
						}
					}

					if !foundSecretKey {
						fmt.Println(kr, krk)
						if deleteSecretKey[kr] == nil {
							deleteSecretKey[kr] = make([]string, 0)
						}
						deleteSecretKey[kr] = append(deleteSecretKey[kr], krk)
						deleteSecretKeyCount++
					}
				}
				break
			}
		}

		if !foundSecret {
			deleteSecret = append(deleteSecret, kr)
			deleteSecretKey[kr] = make([]string, 0)
			for krk := range vr {
				deleteSecretKey[kr] = append(deleteSecretKey[kr], krk)
				deleteSecretKeyCount++
			}
		}
	}

	switch action {
	case "plan":
		if (len(addSecret) > 0) || (addSecretKeyCount > 0) {
			color.Green("Add/Update: %v secret(s) and %v key(s)", len(addSecret), addSecretKeyCount)
			for k, l := range addSecretKey {
				for _, m := range l {
					color.Green("=> %v:%v", k, m)
				}
			}
		}
		if (len(deleteSecret) > 0) || (deleteSecretKeyCount > 0) {
			color.Red("Remove: %v secret(s) and %v key(s)", len(deleteSecret), deleteSecretKeyCount)
			for k, l := range deleteSecretKey {
				for _, m := range l {
					color.Red("=> %v:%v", k, m)
				}
			}
		}
	case "apply":
		for _, k := range addSecret {
			payload := make(map[string]interface{})
			for m, n := range local[k] {
				payload[m] = n
			}
			v.WriteSecret(k, payload)
		}
		for k := range addSecretKey {
			payload := make(map[string]interface{})
			for m, n := range local[k] {
				payload[m] = n
			}
			v.WriteSecret(k, payload)
		}
		for _, k := range deleteSecret {
			v.DeleteSecret(k)
		}
		for k := range deleteSecretKey {
			payload := make(map[string]interface{})
			for m, n := range local[k] {
				payload[m] = n
			}
			v.WriteSecret(k, payload)
		}
	default:
		log.Fatal("No action specified")
		os.Exit(1)
	}
}

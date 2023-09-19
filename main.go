// Command gssh is a wrapper around `gcloud compute ssh` that autocompletes VM names
// and allows for a non-default ssh-username.
//
// Usage:
//
//	# Setup gcloud:
//	gcloud auth login
//	gcloud config set project `foo`
//
//	# Install gssh:
//	go install github.com/corverroos/gssh
//
//	# Setup ssh user via GSSH_USER env var:
//	echo "GSSH_USER=bar" >> ~/.bashrc
//
//	# SSH by selecting one of all VMs:
//	gssh
//
//	# SSH by selecting one of all VMs that start with `foo`:
//	gssh foo
//
//	# SSH to a specific VM:
//	gssh foo-bar
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/manifoldco/promptui"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	flag.Parse()

	var vmFilter string
	if len(os.Args) > 1 {
		vmFilter = os.Args[1]
	}

	var user string
	if u, ok := os.LookupEnv("GSSH_USER"); ok {
		user = u
	}

	err := run(vmFilter, user)
	if err != nil {
		slog.Error("Fatal error: %s", err)
	}
}

func run(vmFilter string, user string) error {
	project, err := getConfig("project")
	if err != nil {
		return err
	}

	fmt.Printf("Defaults: project=%q, user=%q\n", project, user)

	output, err := exec.Command("gcloud", "compute", "instances", "list", "--format=json").CombinedOutput()
	if err != nil {
		return fmt.Errorf("gcloud compute instances list error: %w, %s", err, output)
	}

	var instances []instance
	err = json.Unmarshal(output, &instances)
	if err != nil {
		return fmt.Errorf("unmarshal instances error: %w", err)
	}

	var vms []string
	for _, instance := range instances {
		if !strings.HasPrefix(instance.Name, vmFilter) {
			continue
		}

		label := fmt.Sprintf("%-40s%s", instance.Name, instance.TrimZone())
	
		if vmFilter == instance.Name {
			vms = []string{label}
			break
		}

		vms = append(vms, label)
	}

	var idx int
	if len(vms) == 0 {
		return fmt.Errorf("no VMs found")
	} else if len(vms) == 1 {
		idx = 0
	} else {
		selector := promptui.Select{
			Label: "Select VM",
			Items: vms,
			Size:  len(vms),
		}

		idx, _, err = selector.Run()
		if err != nil {
			return fmt.Errorf("selector error: %w", err)
		}
	}

	zone := instances[idx].TrimZone()
	host := instances[idx].Name
	fmt.Printf("Selected VM: %s (zone=%s)\n", host, zone)
	if user != "" {
		host = user + "@" + host
	}

	cmds := []string{"gcloud", "compute", "ssh", fmt.Sprintf("--zone=%s", zone), host}
	fmt.Printf("Executing: %s\n\n", strings.Join(cmds, " "))

	c := exec.Command(cmds[0], cmds[1:]...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

type instance struct {
	Name string
	Zone string
}

func (i instance) TrimZone() string {
	return filepath.Base(i.Zone)
}

func getConfig(name string) (string, error) {
	output, err := exec.Command("gcloud", "config", "get", name).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gcloud config get %s error: %w, %s", name, err, output)
	}

	return strings.TrimSpace(string(output)), nil
}

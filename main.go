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
	"path"
	"path/filepath"
	"sort"
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
		slog.Error("Fatal error", "err", err)
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

	var prev string
	if conf, err := loadConfig(); err == nil {
		prev = conf.Previous
	}

	var (
		labels     []string
		cursor     int
		selectable []instance
	)
	for _, inst := range sortInstances(instances) {
		if !strings.HasPrefix(inst.Name, vmFilter) {
			continue
		}

		label := fmt.Sprintf("%-40s%s", inst.Name, inst.TrimZone())

		if vmFilter == inst.Name {
			labels = []string{label}
			selectable = []instance{inst}
			break
		}

		labels = append(labels, label)
		selectable = append(selectable, inst)

		if inst.Name == prev {
			cursor = len(selectable) - 1
		}
	}

	var selected int
	if len(selectable) == 0 {
		msg := "no VMs found"
		if vmFilter != "" {
			msg += fmt.Sprintf(" for filter '%s'", vmFilter)
		}
		return fmt.Errorf(msg)
	} else if len(labels) == 1 {
		selected = 0
	} else {
		selector := promptui.Select{
			Label: "Select VM",
			Items: labels,
			Size:  len(labels),
		}

		selected, _, err = selector.RunCursorAt(cursor, 0)
		if err != nil {
			return fmt.Errorf("selector error: %w", err)
		}
	}

	zone := selectable[selected].TrimZone()
	host := selectable[selected].Name
	fmt.Printf("Selected VM: %s (zone=%s)\n", host, zone)

	if err = storeConfig(config{Previous: host}); err != nil {
		slog.Debug("Failed to store config", "err", err)
	}

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

// sortInstances sorts instances by name.
func sortInstances(instances []instance) []instance {
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].Name < instances[j].Name
	})

	return instances
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

func loadConfig() (config, error) {
	filename, ok := configPath()
	if !ok {
		return config{}, fmt.Errorf("HOME env var not present, cannot read config")
	}

	b, err := os.ReadFile(filename)
	if os.IsNotExist(err) {
		return config{}, nil
	} else if err != nil {
		return config{}, fmt.Errorf("read config error: %w", err)
	}

	var conf config
	err = json.Unmarshal(b, &conf)
	if err != nil {
		return config{}, fmt.Errorf("unmarshal config error: %w", err)
	}

	return conf, nil
}

func storeConfig(conf config) error {
	b, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config error: %w", err)
	}

	filename, ok := configPath()
	if !ok {
		return fmt.Errorf("HOME env var not present, cannot store config")
	}

	err = os.WriteFile(filename, b, 0666)
	if err != nil {
		return fmt.Errorf("write config error: %w", err)
	}

	return nil
}

func configPath() (string, bool) {
	home, ok := os.LookupEnv("HOME")
	if !ok {
		return "", false
	}

	return path.Join(home, ".gssh.json"), true
}

type config struct {
	Previous string `json:"previous"`
}

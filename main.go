// Command gssh is a wrapper around `gcloud compute ssh` that autocompletes VM names
// and allows for a non-default ssh-username.
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

const noUserFlag = " "

var (
	flagUser = flag.String("u", noUserFlag, "ssh username (overrides $GSSH_USER env var)")
	flagPrev = flag.Bool("p", false, "use previously selected VM (if any) as filter")
)

func main() {
	flag.Usage = func() {
		o := flag.CommandLine.Output()
		fmt.Fprint(o, "gssh is a wrapper around `gcloud compute ssh` that autocompletes VM names\n")
		fmt.Fprint(o, "\n")
		fmt.Fprintf(o, "Usage: %s [-p] [-u=root] [filter]\n", os.Args[0])
		fmt.Fprint(o, "\n")
		fmt.Fprint(o, "[filter] is a prefix of VM names to filter by\n")
		fmt.Fprint(o, "\n")
		fmt.Fprint(o, "  Flags\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	var vmFilter string
	if len(flag.Args()) > 0 {
		vmFilter = strings.TrimSpace(flag.Args()[0])
	}

	var user string
	if u, ok := os.LookupEnv("GSSH_USER"); ok {
		user = u
	}

	if *flagUser != noUserFlag {
		user = *flagUser
	}

	err := run(vmFilter, user, *flagPrev)
	if err != nil {
		slog.Error("Fatal error", "err", err)
	}
}

// run executes the gssh command.
func run(vmFilter string, user string, usePrev bool) error {
	project, err := getGcloudConfig("project")
	if err != nil {
		return err
	}

	fmt.Printf("Using: project=%q, user=%q, filter=%q, prev=%v\n", project, user, vmFilter, usePrev)

	var prev instance
	if conf, err := loadConfig(); err == nil {
		prev = conf.Previous
	} else if usePrev {
		return fmt.Errorf("cannot connect to previous VM, load config error: %w", err)
	}

	var instances []instance
	if usePrev {
		instances = []instance{prev}
	} else {
		output, err := exec.Command("gcloud", "compute", "instances", "list", "--format=json").CombinedOutput()
		if err != nil {
			return fmt.Errorf("gcloud compute instances list error: %w, %s", err, output)
		}

		err = json.Unmarshal(output, &instances)
		if err != nil {
			return fmt.Errorf("unmarshal instances error: %w", err)
		}

		instances = sortInstances(instances)
	}

	instances = filterInstances(instances, vmFilter)

	if len(instances) == 0 {
		msg := "no VMs found"
		if vmFilter != "" {
			msg += fmt.Sprintf(" for filter '%s'", vmFilter)
		}
		return fmt.Errorf(msg)
	}

	selected := instances[0]
	if len(instances) > 1 {
		selected, err = selectInstance(instances, prev)
		if err != nil {
			return fmt.Errorf("select instance error: %w", err)
		}
	}

	zone := selected.TrimZone()
	host := selected.Name
	fmt.Printf("Selected VM: %s (zone=%s)\n", host, zone)

	if err = storeConfig(config{Previous: selected}); err != nil {
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

// selectInstance prompts the user to select one of the given instances,
// preselecting the previous instance if possible.
func selectInstance(instances []instance, prev instance) (instance, error) {
	var labels []string
	var cursor int
	for i, inst := range instances {
		label := fmt.Sprintf("%-40s%s", inst.Name, inst.TrimZone())

		labels = append(labels, label)

		if inst.Name == prev.Name {
			cursor = i
		}
	}

	selector := promptui.Select{
		Label: "Select VM",
		Items: labels,
		Size:  len(labels),
	}

	idx, _, err := selector.RunCursorAt(cursor, 0)
	if err != nil {
		return instance{}, fmt.Errorf("selector error: %w", err)
	}

	return instances[idx], nil
}

// filterInstances filters instances by name prefix.
func filterInstances(instances []instance, prefix string) []instance {
	if prefix == "" {
		return instances
	}

	var filtered []instance
	for _, inst := range instances {
		if strings.HasPrefix(inst.Name, prefix) {
			filtered = append(filtered, inst)
		}
	}

	return filtered
}

// sortInstances sorts instances by name.
func sortInstances(instances []instance) []instance {
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].Name < instances[j].Name
	})

	return instances
}

// instance is a gcloud compute instance.
type instance struct {
	Name string
	Zone string
}

func (i instance) TrimZone() string {
	return filepath.Base(i.Zone)
}

// getGcloudConfig returns the value of a gcloud config property.
func getGcloudConfig(name string) (string, error) {
	output, err := exec.Command("gcloud", "config", "get", name).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gcloud config get %s error: %w, %s", name, err, output)
	}

	return strings.TrimSpace(string(output)), nil
}

// loadConfig loads the gssh config file.
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

// storeConfig stores the gssh config file.
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

// configPath returns true and the path to the gssh config file or false if
// the HOME env var is not present.
func configPath() (string, bool) {
	home, ok := os.LookupEnv("HOME")
	if !ok {
		return "", false
	}

	return path.Join(home, ".gssh.json"), true
}

// config is the gssh config file format.
type config struct {
	Previous instance `json:"previous"`
}

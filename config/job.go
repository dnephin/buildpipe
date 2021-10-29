package config

import (
	"fmt"
	"os"
	"reflect"

	"github.com/dnephin/configtf"
	pth "github.com/dnephin/configtf/path"
	"github.com/dnephin/dobi/tasks/task"
	shlex "github.com/kballard/go-shellquote"
	"golang.org/x/term"
)

// JobConfig A **job** resource uses an `image`_ to run a job in a container.
//
// A **job** resource that doesn't have an ``artifact`` is never considered
// up-to-date and will always run.  If a job resource has an ``artifact``
// the job will be skipped if the artifact is newer than the source.
// The last modified time of the ``artifact`` files is compared against the
// last modified time of the files in ``sources``, or if ``sources`` is left
// unset, the last modified time of the ``use`` image and all the files in
// the ``mounts``.
//
// ``mounts`` are provided to the container as bind mounts. If the ``DOBI_NO_BIND_MOUNT``
// environment variable, or `--no-bind-mount` flag is set, then ``mounts``
// will be copied into the container, and all artifacts will be copied out of the
// container to the host after the job is complete.
//
// The `image`_ specified in ``use`` and any `mount`_ resources listed in
// ``mounts`` are automatically added as dependencies and will always be
// created first.
//
// name: job
// example: Run a container using the ``builder`` image to compile some source
// code to ``./dist/app-binary``.
//
// .. code-block:: yaml
//
//     job=compile:
//         use: builder
//         mounts: [source, dist]
//         artifact: dist/app-binary
//
type JobConfig struct {
	// Use The name of an `image`_ resource. The referenced image is used
	// to created the container for the **job**.
	Use string `config:"required"`
	// Artifact File paths or globs identifying the files created by the **job**.
	// Paths to directories must end with a path separator (``/``).
	// Paths are relative to the ``dobi.yaml``
	// type: list of file paths or glob patterns
	Artifact PathGlobs
	// Command The command to run in the container.
	// type: shell quoted string
	// example: ``"bash -c 'echo something'"``
	Command ShlexSlice
	// Entrypoint Override the image entrypoint
	// type: shell quoted string
	Entrypoint ShlexSlice
	// Sources File paths or globs of the files used to create the
	// artifact. The modified time of these files are compared to the modified time
	// of the artifact to determine if the **job** is stale. If the **sources**
	// list is defined the modified time of **mounts** and the **use** image are
	// ignored.
	// type: list of file paths or glob patterns
	Sources PathGlobs
	// Mounts A list of `mount`_ resources to use when creating the container.
	// type: list of mount resources
	Mounts []string
	// Privileged Gives extended privileges to the container
	Privileged bool
	// Interactive Makes the container interative and enables a tty.
	Interactive bool
	// Env Environment variables to pass to the container. This field
	// supports :doc:`variables`.
	// type: list of ``key=value`` strings
	Env []string
	// ProvideDocker Exposes the docker engine to the container by either
	// mounting the unix socket or setting the ``DOCKER_HOST`` environment
	// variable. All environment variables with a  ``DOCKER_`` prefix in the
	// environment are set on the container.
	ProvideDocker bool
	// NetMode The network mode to use. This field supports :doc:`variables`.
	NetMode string
	// WorkingDir The directory to set as the active working directory in the
	// container. This field supports :doc:`variables`.
	WorkingDir string
	// User Username or UID to use in the container. Format ``user[:group]``.
	User string
	// Ports Publish ports to the host
	// type: list of 'host_port:container_port'
	Ports []string
	// Devices Maps the host devices you want to connect to a container
	// type: list of device specs
	// example: ``{Host: /dev/fb0, Container: /dev/fb0, Permissions: rwm}``
	Devices []Device
	// Labels sets the labels of the running job container
	// type: map of string keys to string values
	Labels map[string]string
	Dependent
	Annotations
}

// Device is the defined structure to attach host devices to containers
type Device struct {
	Host        string
	Container   string
	Permissions string
}

// Dependencies returns the list of implicit and explicit dependencies
func (c *JobConfig) Dependencies() ([]task.Name, error) {
	deps, err := task.ParseNames(c.Depends)
	if err != nil {
		return []task.Name{}, err
	}
	mnts, err := task.ParseNames(c.Mounts)
	if err != nil {
		return []task.Name{}, err
	}
	use, err := task.ParseName(c.Use)
	if err != nil {
		return []task.Name{}, err
	}
	return append(mnts, append(deps, use)...), nil
}

// Validate checks that all fields have acceptable values
func (c *JobConfig) Validate(path pth.Path, config *Config) *pth.Error {
	validators := []validator{
		newValidator("use", func() error { return c.validateUse(config) }),
		newValidator("mounts", func() error { return c.validateMounts(config) }),
		newValidator("artifact", c.Artifact.Validate),
		newValidator("sources", c.Sources.Validate),
	}
	for _, validator := range validators {
		if err := validator.validate(); err != nil {
			return pth.Errorf(path.Add(validator.name), err.Error())
		}
	}
	return nil
}

func (c *JobConfig) validateUse(config *Config) error {
	err := fmt.Errorf("%s is not an image resource", c.Use)

	res, ok := config.Resources[c.Use]
	if !ok {
		return err
	}

	switch res.(type) {
	case *ImageConfig:
	default:
		return err
	}

	return nil
}

func (c *JobConfig) validateMounts(config *Config) error {
	for _, mount := range c.Mounts {
		err := fmt.Errorf("%s is not a mount resource", mount)

		res, ok := config.Resources[mount]
		if !ok {
			return err
		}

		switch res.(type) {
		case *MountConfig:
		default:
			return err
		}
	}
	return nil
}

func (c *JobConfig) String() string {
	artifact, command := "", ""
	if !c.Artifact.Empty() {
		artifact = fmt.Sprintf(" to create '%s'", &c.Artifact)
	}
	// TODO: look for entrypoint as well as command
	if !c.Command.Empty() {
		command = fmt.Sprintf("'%s' using ", c.Command.String())
	}
	return fmt.Sprintf("Run %sthe '%s' image%s", command, c.Use, artifact)
}

// Resolve resolves variables in the resource
func (c *JobConfig) Resolve(resolver Resolver) (Resource, error) {
	conf := *c
	var err error
	conf.Env, err = resolver.ResolveSlice(c.Env)
	if err != nil {
		return &conf, err
	}
	conf.WorkingDir, err = resolver.Resolve(c.WorkingDir)
	if err != nil {
		return &conf, err
	}
	conf.User, err = resolver.Resolve(c.User)
	if err != nil {
		return &conf, err
	}
	conf.NetMode, err = resolver.Resolve(c.NetMode)
	return &conf, err
}

// ShlexSlice is a type used for config transforming a string into a []string
// using shelx.
type ShlexSlice struct {
	original string
	parsed   []string
}

func (s *ShlexSlice) String() string {
	return s.original
}

// Value returns the slice value
func (s *ShlexSlice) Value() []string {
	return s.parsed
}

// Empty returns true if the instance contains the zero value
func (s *ShlexSlice) Empty() bool {
	return s.original == ""
}

// TransformConfig is used to transform a string from a config file into a
// sliced value, using shlex.
func (s *ShlexSlice) TransformConfig(raw reflect.Value) error {
	if !raw.IsValid() {
		return fmt.Errorf("must be a string, was undefined")
	}

	var err error
	switch value := raw.Interface().(type) {
	case string:
		s.original = value
		s.parsed, err = shlex.Split(value)
		if err != nil {
			return fmt.Errorf("failed to parse command %q: %s", value, err)
		}
	default:
		return fmt.Errorf("must be a string, not %T", value)
	}
	return nil
}

func jobFromConfig(name string, values map[string]interface{}) (Resource, error) {
	isTerminal := term.IsTerminal(int(os.Stdin.Fd()))
	cmd := &JobConfig{}
	if isTerminal {
		if _, ok := values["interactive"]; !ok {
			values["interactive"] = true
		}
	}
	return cmd, configtf.Transform(name, values, cmd)
}

func init() {
	RegisterResource("job", jobFromConfig)
	// Backwards compatibility for v0.4, remove in v1.0
	RegisterResource("run", jobFromConfig)
}

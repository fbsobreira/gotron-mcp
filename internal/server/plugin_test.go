package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/fbsobreira/gotron-mcp/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The marketplace installs each plugin directory on its own (a sparse
// checkout), so gotron-local carries copies of the hosted plugin's skills.
const (
	hostedPluginDir = "../../plugin"
	localPluginDir  = "../../plugin-local"
)

// policyOnlyTools are registered only when a wallet policy is loaded, which
// the keystore server below doesn't configure.
var policyOnlyTools = map[string]bool{
	"request_limit_override": true,
}

// skillToolVerbs are the tool verbs the skills use. They are fixed here, not
// only derived from the registry, so renaming every tool with a verb leaves
// the old names recognized as tool references and failing the check.
var skillToolVerbs = []string{
	"analyze", "delegate", "estimate", "freeze", "get", "is", "list",
	"request", "sign", "transfer", "validate",
}

var (
	codeSpanRe    = regexp.MustCompile("`([^`]+)`")
	snakeCaseRe   = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)+$`)
	frontmatterRe = regexp.MustCompile(`(?s)\A---\n(.*?)\n---\n`)
)

type pluginManifest struct {
	Name       string `json:"name"`
	MCPServers map[string]struct {
		Type    string            `json:"type"`
		URL     string            `json:"url"`
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
	} `json:"mcpServers"`
}

func readPluginManifest(t *testing.T, dir string) pluginManifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	require.NoError(t, err)
	var m pluginManifest
	require.NoError(t, json.Unmarshal(data, &m))
	return m
}

func TestPluginManifest_Hosted(t *testing.T) {
	m := readPluginManifest(t, hostedPluginDir)
	assert.Equal(t, "gotron", m.Name)
	require.Contains(t, m.MCPServers, "gotron")
	assert.Equal(t, "http", m.MCPServers["gotron"].Type)
	assert.Equal(t, "https://mcp.gotron.sh/mcp", m.MCPServers["gotron"].URL)
}

func TestPluginManifest_Local(t *testing.T) {
	m := readPluginManifest(t, localPluginDir)
	assert.Equal(t, "gotron-local", m.Name)
	require.Contains(t, m.MCPServers, "gotron")
	srv := m.MCPServers["gotron"]
	assert.Equal(t, "gotron-mcp", srv.Command, "must run the binary from PATH; the plugin directory has no binary")
	assert.Equal(t, []string{"--transport", "stdio"}, srv.Args)
	assert.Equal(t, map[string]string{
		"GOTRON_MCP_NETWORK":             "${user_config.network}",
		"GOTRON_MCP_KEYSTORE_PASSPHRASE": "${user_config.keystore_passphrase}",
		"GOTRON_MCP_KEYSTORE_DIR":        "${user_config.keystore_dir}",
		"GOTRON_NODE_API_KEY":            "${user_config.api_key}",
	}, srv.Env)
}

func TestPluginLocalSkillsMatchHosted(t *testing.T) {
	local, err := filepath.Glob(filepath.Join(localPluginDir, "skills", "*", "SKILL.md"))
	require.NoError(t, err)
	require.NotEmpty(t, local)

	for _, path := range local {
		dir := filepath.Base(filepath.Dir(path))
		hosted := filepath.Join(hostedPluginDir, "skills", dir, "SKILL.md")
		want, err := os.ReadFile(hosted)
		if os.IsNotExist(err) {
			// A skill only local mode needs lives in plugin-local alone.
			continue
		}
		require.NoError(t, err)
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, string(want), string(got), "%s differs from %s; copy the hosted skill over", path, hosted)
	}

	hostedSkills, err := filepath.Glob(filepath.Join(hostedPluginDir, "skills", "*", "SKILL.md"))
	require.NoError(t, err)
	for _, path := range hostedSkills {
		dir := filepath.Base(filepath.Dir(path))
		assert.FileExists(t, filepath.Join(localPluginDir, "skills", dir, "SKILL.md"), "gotron-local is missing skill %s", dir)
	}
}

// TestPluginSkillsReferenceRegisteredTools fails when a skill names a tool
// the server doesn't register, so renaming a tool breaks CI until the
// skills are updated. A code span counts as a tool reference when it is
// snake_case and starts with a tool verb (get_, sign_, ...), which keeps
// output fields such as `transaction_hex` out of the check.
func TestPluginSkillsReferenceRegisteredTools(t *testing.T) {
	cfg := &config.Config{
		Transport:    "stdio",
		Network:      "mainnet",
		Node:         "mock:50051",
		KeystoreDir:  t.TempDir(),
		KeystorePass: "test-pass",
	}
	s, wm, _ := New(cfg, newTestPool())
	require.NotNil(t, wm)
	t.Cleanup(wm.Close)

	registered := s.ListTools()
	verbs := map[string]bool{}
	for _, v := range skillToolVerbs {
		verbs[v] = true
	}
	for name := range registered {
		verbs[strings.SplitN(name, "_", 2)[0]] = true
	}
	for name := range policyOnlyTools {
		verbs[strings.SplitN(name, "_", 2)[0]] = true
	}

	for _, pluginDir := range []string{hostedPluginDir, localPluginDir} {
		skills, err := filepath.Glob(filepath.Join(pluginDir, "skills", "*", "SKILL.md"))
		require.NoError(t, err)
		require.NotEmpty(t, skills, "no skills found under %s", pluginDir)

		for _, path := range skills {
			dir := filepath.Base(filepath.Dir(path))
			t.Run(filepath.Base(pluginDir)+"/"+dir, func(t *testing.T) {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				body := string(data)

				fm := frontmatterRe.FindStringSubmatch(body)
				require.NotNil(t, fm, "missing YAML frontmatter")
				assert.Contains(t, fm[1], "name: "+dir+"\n", "frontmatter name must match the directory")
				assert.Regexp(t, `(?m)^description: \S`, fm[1], "frontmatter needs a description")

				refs := 0
				for _, m := range codeSpanRe.FindAllStringSubmatch(body, -1) {
					token := m[1]
					if !snakeCaseRe.MatchString(token) || !verbs[strings.SplitN(token, "_", 2)[0]] {
						continue
					}
					refs++
					if policyOnlyTools[token] {
						continue
					}
					_, ok := registered[token]
					assert.True(t, ok, "skill references unregistered tool %q", token)
				}
				assert.NotZero(t, refs, "skill references no tools")
			})
		}
	}
}

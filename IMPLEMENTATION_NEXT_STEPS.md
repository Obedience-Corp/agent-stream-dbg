# Stream-Debugger: Remaining Implementation Steps

## ✅ Completed Tasks

### Backend (BrainyardV3)
1. **API Key Authentication System** ✅
   - Created `app/models/api_key.py` with APIKey model
   - Added API key authentication to `app/auth.py`
   - Added `require_auth_or_api_key` decorator
   - Updated streaming endpoint to accept API key auth

2. **Session Management Endpoint** ✅
   - Created `app/api/debug.py` with debug endpoints
   - `POST /api/v3/debug/session` - Create/get session with default agents
   - `GET /api/v3/debug/session/<id>` - Get session info
   - `DELETE /api/v3/debug/session/<id>` - Delete session

3. **Configuration** ✅
   - Fixed `configs/brainyard-v3.yaml` to use POST + JSON body
   - Added auto-setup configuration with default agents

## ⏳ Remaining Tasks

### 1. Database Migration for APIKey Model
**Priority: HIGH** - Required before API keys can be used

**Location:** `BrainyardV3/backend/migrations/`

**Steps:**
```bash
cd BrainyardV3/backend
flask db migrate -m "Add APIKey model for tool authentication"
flask db upgrade
```

**What the migration should include:**
- Create `api_keys` table with all columns from APIKey model
- Foreign key to `users` table
- Indexes on `key_hash` and `key_prefix`

### 2. CLI Command to Generate API Keys
**Priority: HIGH** - Users need a way to create API keys

**Location:** `BrainyardV3/backend/app/cli/api_keys.py` (new file)

**Implementation:**
```python
import click
from flask.cli import with_appcontext
from app.extensions import db
from app.models import APIKey, User

@click.group()
def api_key():
    """Manage API keys for external tools"""
    pass

@api_key.command()
@click.argument('user_email')
@click.option('--name', default='Debug Tool', help='API key name')
@click.option('--description', help='API key description')
@with_appcontext
def generate(user_email, name, description):
    """Generate a new API key for a user"""
    user = User.query.filter_by(email=user_email).first()

    if not user:
        click.echo(f"Error: User {user_email} not found")
        return

    # Generate key
    key = APIKey.generate_key()
    key_prefix = key[:18]  # 'brainyard_' + 8 chars

    # Create API key object
    api_key = APIKey(
        key_hash=APIKey.hash_key(key),
        key_prefix=key_prefix,
        name=name,
        description=description,
        user_id=user.id,
        active=True
    )

    db.session.add(api_key)
    db.session.commit()

    click.echo(f"\n✅ API Key generated successfully!")
    click.echo(f"   Name: {name}")
    click.echo(f"   User: {user_email}")
    click.echo(f"\n   API Key: {key}")
    click.echo(f"\n⚠️  Save this key now - it won't be shown again!")
    click.echo(f"   Add to stream-debugger/.env: API_KEY={key}\n")

def register_api_key_commands(app):
    app.cli.add_command(api_key)
```

**Register in `app/__init__.py`:**
```python
from app.cli.api_keys import register_api_key_commands
register_api_key_commands(app)
```

**Usage:**
```bash
cd BrainyardV3
flask api-key generate user@example.com --name "Stream Debugger" --description "Local debugging tool"
```

**Or add to justfile:**
```justfile
# Generate API key for debug tools
generate-api-key email:
    cd backend && uv run flask api-key generate {{email}} --name "Stream Debugger"
```

### 3. Interactive TUI Mode with Input Box
**Priority: HIGH** - Core feature for usability

**Location:** `stream-debugger/internal/visualizer/tui.go`

**Current:** TUI displays streaming responses but takes message as CLI arg
**Needed:** Input box at bottom for typing messages interactively

**Implementation Approach:**

Use Bubbletea's textarea component for input. The TUI needs two modes:
1. **View Mode** - Display agent responses (existing)
2. **Input Mode** - User typing message

**Key Changes Needed:**

```go
// Add to Model struct:
type Model struct {
    // ... existing fields ...
    inputBox     textarea.Model
    inputMode    bool // true when user is typing
    messages     []string // History of sent messages
}

// Update Init():
func (m Model) Init() tea.Cmd {
    m.inputBox = textarea.New()
    m.inputBox.Placeholder = "Type your message and press Enter to send..."
    m.inputBox.Focus()
    return tea.Batch(
        textarea.Blink,
        m.listenForEvents(), // Start listening for SSE
    )
}

// Update View() to show input box at bottom:
func (m Model) View() string {
    // ... agent panels ...

    // Input box at bottom
    inputView := lipgloss.NewStyle().
        Border(lipgloss.RoundedBorder()).
        BorderForeground(lipgloss.Color("62")).
        Padding(0, 1).
        Render(m.inputBox.View())

    return lipgloss.JoinVertical(
        lipgloss.Left,
        agentPanels,
        inputView,
    )
}

// Update Update() to handle input:
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        if m.inputMode {
            switch msg.String() {
            case "enter":
                // Send message
                message := m.inputBox.Value()
                if message != "" {
                    m.inputBox.Reset()
                    return m, m.sendMessage(message)
                }
            case "esc":
                m.inputMode = false
            default:
                var cmd tea.Cmd
                m.inputBox, cmd = m.inputBox.Update(msg)
                return m, cmd
            }
        } else {
            switch msg.String() {
            case "i":
                m.inputMode = true
                m.inputBox.Focus()
            case "q", "ctrl+c":
                return m, tea.Quit
            }
        }
    }
    // ... handle SSE events ...
}
```

### 4. Auto-Setup Flow for Session Creation
**Priority: HIGH** - Required for simplified workflow

**Location:** `stream-debugger/internal/client/session_setup.go` (new file)

**Implementation:**

```go
package client

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "github.com/lancekrogers/stream-debugger/internal/config"
)

type SessionSetupClient struct {
    config *config.Config
    apiKey string
}

type SessionSetupRequest struct {
    SessionID     string   `json:"session_id,omitempty"`
    Agents        []string `json:"agents"`
    ReuseExisting bool     `json:"reuse_existing"`
}

type SessionSetupResponse struct {
    Success      bool     `json:"success"`
    SessionID    string   `json:"session_id"`
    ActiveAgents []string `json:"active_agents"`
    Created      bool     `json:"created"`
    Message      string   `json:"message"`
}

func NewSessionSetupClient(cfg *config.Config, apiKey string) *SessionSetupClient {
    return &SessionSetupClient{
        config: cfg,
        apiKey: apiKey,
    }
}

func (c *SessionSetupClient) CreateOrGetSession() (*SessionSetupResponse, error) {
    url := fmt.Sprintf("%s%s",
        c.config.Backend.BaseURL,
        c.config.Session.SetupEndpoint,
    )

    request := SessionSetupRequest{
        SessionID:     c.config.Session.ID,
        Agents:        c.config.Session.DefaultAgents,
        ReuseExisting: true,
    }

    jsonData, err := json.Marshal(request)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal request: %w", err)
    }

    req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("failed to send request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
        return nil, fmt.Errorf("session setup failed with status: %d", resp.StatusCode)
    }

    var result SessionSetupResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("failed to decode response: %w", err)
    }

    return &result, nil
}
```

**Update `cmd/stream-debugger/main.go` to call auto-setup before starting TUI**

### 5. Simplify Command to `stream-debugger --config`
**Priority: MEDIUM** - UX improvement

**Location:** `stream-debugger/cmd/stream-debugger/main.go`

**Current:**
```bash
stream-debugger stream "message" --config file.yaml
stream-debugger replay logs/session.jsonl
stream-debugger timeline logs/session.jsonl
```

**Desired:**
```bash
stream-debugger --config file.yaml  # Interactive TUI
stream-debugger replay logs/session.jsonl
stream-debugger timeline logs/session.jsonl
```

**Changes Needed:**

```go
func main() {
    configPath := flag.String("config", "config.yaml", "Path to configuration file")
    flag.Parse()

    args := flag.Args()

    // If no command, default to interactive TUI
    if len(args) == 0 {
        startInteractiveTUI(*configPath)
        return
    }

    command := args[0]

    switch command {
    case "replay":
        if len(args) < 2 {
            fmt.Println("Usage: stream-debugger replay <session_file>")
            os.Exit(1)
        }
        replaySession(args[1])
    case "timeline":
        if len(args) < 2 {
            fmt.Println("Usage: stream-debugger timeline <session_file>")
            os.Exit(1)
        }
        showTimeline(args[1])
    default:
        fmt.Printf("Unknown command: %s\n", command)
        showUsage()
        os.Exit(1)
    }
}

func startInteractiveTUI(configPath string) {
    // Load config
    cfg, err := config.LoadConfig(configPath)
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    // Get API key from env
    apiKey := os.Getenv("API_KEY")
    if apiKey == "" {
        log.Fatal("API_KEY environment variable not set")
    }

    // Auto-setup session if configured
    if cfg.Session.AutoSetup {
        setupClient := client.NewSessionSetupClient(cfg, apiKey)
        session, err := setupClient.CreateOrGetSession()
        if err != nil {
            log.Fatalf("Failed to setup session: %v", err)
        }
        fmt.Printf("✅ Session ready: %s (agents: %v)\n",
            session.SessionID, session.ActiveAgents)

        // Update config with actual session ID
        cfg.Session.ID = session.SessionID
    }

    // Start interactive TUI
    m := visualizer.NewModel(cfg, apiKey)
    p := tea.NewProgram(m, tea.WithAltScreen())
    if err := p.Start(); err != nil {
        log.Fatalf("TUI error: %v", err)
    }
}
```

### 6. Update Documentation
**Priority: MEDIUM** - Help users get started

**Files to Update:**
- `README.md` - Quick start with new workflow
- `docs/user-guide/getting-started-brainyard.md` - Complete setup guide
- `.env.example` - Add API_KEY example

**README Quick Start Section:**
```markdown
## Quick Start with BrainyardV3

1. **Start BrainyardV3 Backend**
   ```bash
   cd BrainyardV3
   just up
   ```

2. **Generate API Key**
   ```bash
   just generate-api-key your-email@example.com
   # Copy the generated API key
   ```

3. **Configure stream-debugger**
   ```bash
   cd ../stream-debugger
   echo "API_KEY=brainyard_..." > .env
   ```

4. **Launch Interactive Debugger**
   ```bash
   stream-debugger --config configs/brainyard-v3.yaml
   ```

5. **Type messages and see live streaming!**
   - Type your message in the input box
   - Press Enter to send
   - Watch multiple agents respond in real-time
   - Press 'q' to quit
```

## Testing Checklist

Before considering this feature complete, test:

- [ ] Database migration runs successfully
- [ ] API key generation works
- [ ] API key authentication works with streaming endpoint
- [ ] Debug session endpoint creates session with agents
- [ ] stream-debugger can connect with API key
- [ ] Interactive TUI opens and displays input box
- [ ] Can type and send multiple messages
- [ ] Agent responses stream correctly in TUI
- [ ] Can quit cleanly with 'q' or Ctrl+C
- [ ] Logs are written correctly to all 4 dimensions

## Estimated Complexity

**Remaining Go Changes:**
- Interactive TUI: ~200-300 lines (medium complexity)
- Session setup client: ~100 lines (easy)
- CLI simplification: ~50-100 lines (easy)
- Total: ~350-500 lines of Go code

**Remaining Python Changes:**
- Migration: Auto-generated
- CLI command: ~60 lines (easy)
- Justfile: ~5 lines (trivial)

**Total Time Estimate:** 2-3 hours for experienced Go developer

## Priority Order

1. Database migration + API key generation (required for testing)
2. CLI command simplification (easier, immediate UX improvement)
3. Auto-setup flow (enables end-to-end testing)
4. Interactive TUI (most complex, but most valuable)
5. Documentation updates (polish)

# Evaluation: Init Command UX Improvements
Generated: 2026-01-07

## Topic
Improve the `chaperone init` wizard UX by adding explanatory text and context.

## Current State

### What Exists
The init wizard is implemented in:
- `internal/init/wizard.go` - Main wizard orchestration (483 lines)
- `cmd/chaperone/cmd/init.go` - Cobra command and CLI entry point (257 lines)

**Current wizard flow:**
1. `Step1ConfigureProxy()` - Prompts for address, port, sentinel value
2. `PrintDetectionInstructions()` - Minimal instructions for detection mode
3. `ReportFinding()` - Real-time finding display
4. `Step3ReviewFindings()` - Lists detected services
5. `Step4ConfigureService()` - Prompts for service name, credential, storage type
6. `Step5SaveConfig()` - Save location choice and TOML generation
7. `printNextSteps()` - Post-wizard instructions

**Current prompts (no explanations):**
- "Listen address [127.0.0.1]: "
- "Listen port [4010]: "
- "Sentinel value (optional, press Enter to skip): "
- "Service name [openai]: "
- "Enter the API key/credential value: "
- "Where do you want to store this credential?" (1/2/3 choices)
- "Where do you want to save the configuration?" (1/2/3/4 choices)

### What's Missing
1. **No introduction** - Users jump straight into "Step 1: Configure Proxy" without understanding what Chaperone does or why they'd want it
2. **No overview** - No bullet point summary of what the init process will accomplish
3. **No context on questions** - Each prompt lacks a brief explanation of what the value is for

### What Needs Changes

**File: `internal/init/wizard.go`**

1. Add new `PrintIntroduction()` method that outputs:
   - A paragraph explaining what Chaperone does (MITM proxy, credential injection, app never sees API keys)
   - Bullet point overview of init process steps

2. Update `Step1ConfigureProxy()`:
   - Add brief help text for "Listen address" (where proxy binds)
   - Add brief help text for "Listen port" (proxy port)
   - Add brief help text for "Sentinel value" (exact string to search for in requests)

3. Update `Step4ConfigureService()`:
   - Add brief help text for "Service name" (identifier in config file)
   - Add brief help text for "API key/credential value" (the actual secret to inject)
   - Add brief help text for storage options (security characteristics of each)

4. Update `Step5SaveConfig()`:
   - Add brief help text for save location options

**File: `cmd/chaperone/cmd/init.go`**

5. Call `wizard.PrintIntroduction()` before Step 1

## Dependencies
- None - this is purely additive UX text

## Risks
- Text could be too verbose, making wizard feel slow
- Help text formatting could break terminal width expectations

## Verdict
**CONTINUE** - Requirements are clear, implementation is straightforward.

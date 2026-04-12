# DeployMonster — BRANDING.md

> **Product**: DeployMonster
> **Tagline**: "Tame Your Deployments"
> **Domains**: deploy.monster · deploymonster.com
> **GitHub**: github.com/deploy-monster

---

## 1. BRAND IDENTITY

### 1.1 Name & Story

**DeployMonster** — the name communicates two things:

1. **Deploy** — the core action, what the product does
2. **Monster** — powerful, unstoppable, mythical creature that handles everything

The "Monster" is friendly but fierce — like a guardian creature that tames the chaos of infrastructure. Think Sully from Monsters Inc. meets DevOps.

### 1.2 Tagline Options

| Primary | Context |
|---------|---------|
| **"Tame Your Deployments"** | Main tagline — hero section, logo lockup |
| **"Deploy Anything. Manage Everything."** | Feature-focused — landing page |
| **"Your Infrastructure, Unleashed."** | Power-focused — enterprise marketing |
| **"One Binary. Infinite Possibilities."** | Technical — developer marketing |
| **"The Self-Hosted PaaS That Bites Back."** | Playful — social media, community |

### 1.3 Brand Voice

| Attribute | Description | Example |
|-----------|-------------|---------|
| **Confident** | We know we're better than Coolify/Dokploy | "Not another deploy toy. A real platform." |
| **Developer-first** | Speak their language, no marketing BS | "Single binary. Zero config. `./deploymonster` and you're live." |
| **Playful** | Monster theme allows fun without being childish | "Feed your monster a docker-compose.yml. Watch it deploy." |
| **Direct** | No buzzwords, no fluff | "150+ one-click apps. Drag & drop topology. Built-in billing." |
| **Open** | Open source pride, community-driven | "AGPL core. Free forever. Enterprise when you need it." |

---

## 2. LOGO SYSTEM

### 2.1 Logo Concept

The DeployMonster logo is a **stylized monster face/head** combined with deployment imagery:

**Primary Mark — "Monster Icon":**
- A friendly but powerful monster head silhouette
- Subtle container/box shapes integrated into the design (shipping container = Docker container metaphor)
- One eye or horn detail that references a rocket/deploy arrow
- Works at 16×16 favicon size and 512×512 app icon
- Geometric, modern, works in single color

**Logo Variations:**

| Variant | Usage | Format |
|---------|-------|--------|
| **Icon Only** | Favicon, app icon, small spaces | SVG, PNG (16/32/64/128/256/512) |
| **Icon + Wordmark** | Header, marketing, README | SVG, PNG |
| **Wordmark Only** | Text-heavy contexts, CLI | SVG |
| **Icon + Wordmark + Tagline** | Landing page hero | SVG, PNG |

### 2.2 Logo Color Variants

| Context | Background | Logo Color |
|---------|-----------|------------|
| Dark mode (primary) | Dark (#0A0F1E) | Monster Green (#22C55E) |
| Light mode | White (#FFFFFF) | Dark (#0A0F1E) |
| On green background | Monster Green | White (#FFFFFF) |
| Monochrome | Any | Single color (black or white) |

### 2.3 Logo Clear Space

Minimum clear space around the logo = height of the "D" in DeployMonster on all sides.

### 2.4 Logo Don'ts

- Don't stretch or distort
- Don't change colors outside brand palette
- Don't add shadows, gradients, or 3D effects
- Don't place on busy backgrounds without contrast
- Don't rotate
- Don't use the old/alternate versions after brand finalization

---

## 3. COLOR SYSTEM

### 3.1 Primary Palette

| Token | Name | Hex | HSL | Usage |
|-------|------|-----|-----|-------|
| `--monster-green` | Monster Green | `#22C55E` | `142 76% 46%` | Primary brand color, CTAs, success states |
| `--monster-purple` | Monster Purple | `#8B5CF6` | `262 83% 58%` | Accent, highlights, premium features |
| `--monster-dark` | Monster Dark | `#0A0F1E` | `224 54% 8%` | Dark backgrounds, dark mode base |
| `--monster-white` | Monster White | `#F8FAFC` | `210 40% 98%` | Light backgrounds, light mode base |

### 3.2 Extended Palette

| Token | Name | Hex | Usage |
|-------|------|-----|-------|
| `--green-50` | Green 50 | `#F0FDF4` | Light green tint |
| `--green-100` | Green 100 | `#DCFCE7` | Success background |
| `--green-500` | Green 500 | `#22C55E` | Primary (= Monster Green) |
| `--green-600` | Green 600 | `#16A34A` | Primary hover |
| `--green-700` | Green 700 | `#15803D` | Primary active |
| `--purple-50` | Purple 50 | `#FAF5FF` | Light purple tint |
| `--purple-500` | Purple 500 | `#8B5CF6` | Accent (= Monster Purple) |
| `--purple-600` | Purple 600 | `#7C3AED` | Accent hover |
| `--red-500` | Red 500 | `#EF4444` | Error, destructive |
| `--yellow-500` | Yellow 500 | `#EAB308` | Warning |
| `--blue-500` | Blue 500 | `#3B82F6` | Info, links (light mode) |

### 3.3 Dark Mode Palette (UI)

```css
:root[data-theme="dark"] {
  --background: 224 54% 5%;        /* #090D19 - page bg */
  --foreground: 210 20% 98%;       /* #F8FAFC - primary text */
  --card: 224 40% 8%;              /* #0F1524 - card surface */
  --card-foreground: 210 20% 98%;
  --popover: 224 40% 8%;
  --popover-foreground: 210 20% 98%;
  --primary: 142 76% 46%;          /* #22C55E - Monster Green */
  --primary-foreground: 0 0% 100%;
  --secondary: 215 25% 15%;        /* #1E293B - secondary bg */
  --secondary-foreground: 210 20% 90%;
  --muted: 215 25% 20%;            /* #2D3A4E */
  --muted-foreground: 215 20% 65%;
  --accent: 262 83% 58%;           /* #8B5CF6 - Monster Purple */
  --accent-foreground: 0 0% 100%;
  --destructive: 0 84% 60%;        /* #EF4444 */
  --destructive-foreground: 0 0% 100%;
  --border: 215 25% 18%;           /* #252F3F */
  --input: 215 25% 18%;
  --ring: 142 76% 46%;             /* Green focus ring */
  --sidebar: 224 50% 5%;           /* #080C17 - sidebar bg */
  --sidebar-foreground: 210 20% 80%;
  --sidebar-accent: 142 76% 46%;
}
```

### 3.4 Light Mode Palette (UI)

```css
:root[data-theme="light"] {
  --background: 0 0% 100%;         /* #FFFFFF */
  --foreground: 224 54% 8%;        /* #0A0F1E */
  --card: 0 0% 100%;
  --card-foreground: 224 54% 8%;
  --primary: 142 76% 36%;          /* #16A34A - slightly darker for contrast */
  --primary-foreground: 0 0% 100%;
  --secondary: 210 40% 96%;        /* #F1F5F9 */
  --secondary-foreground: 215 25% 27%;
  --muted: 210 40% 96%;
  --muted-foreground: 215 16% 47%;
  --accent: 262 83% 58%;           /* #8B5CF6 */
  --accent-foreground: 0 0% 100%;
  --destructive: 0 84% 60%;
  --border: 214 32% 91%;           /* #E2E8F0 */
  --input: 214 32% 91%;
  --ring: 142 76% 36%;
  --sidebar: 210 40% 98%;          /* #F8FAFC */
  --sidebar-foreground: 224 54% 8%;
  --sidebar-accent: 142 76% 36%;
}
```

### 3.5 Semantic Colors

| Semantic | Color | Usage |
|----------|-------|-------|
| Success | Monster Green `#22C55E` | Healthy, running, deployed, connected |
| Warning | Yellow `#EAB308` | Degraded, approaching limit, expiring |
| Error | Red `#EF4444` | Failed, crashed, down, exceeded |
| Info | Blue `#3B82F6` | Building, deploying, processing |
| Neutral | Gray `#94A3B8` | Disabled, unknown, not deployed |
| Premium | Monster Purple `#8B5CF6` | Enterprise features, upgrade prompts |

### 3.6 Status Color Mapping

| Status | Color | Badge | Dot |
|--------|-------|-------|-----|
| Running | Green | `bg-green-500/10 text-green-500` | 🟢 |
| Building | Blue | `bg-blue-500/10 text-blue-500` | 🔵 |
| Deploying | Blue | `bg-blue-500/10 text-blue-500` | 🔵 |
| Stopped | Gray | `bg-gray-500/10 text-gray-500` | ⚪ |
| Crashed | Red | `bg-red-500/10 text-red-500` | 🔴 |
| Failed | Red | `bg-red-500/10 text-red-500` | 🔴 |
| Degraded | Yellow | `bg-yellow-500/10 text-yellow-500` | 🟡 |
| Scaling | Orange | `bg-orange-500/10 text-orange-500` | 🟠 |
| Pending | Gray | `bg-gray-500/10 text-gray-400` | ⚪ |

---

## 4. TYPOGRAPHY

### 4.1 Font Stack

| Context | Font | Weight | Fallback |
|---------|------|--------|----------|
| **Headings** | Inter | 600 (Semibold), 700 (Bold) | system-ui, -apple-system, sans-serif |
| **Body** | Inter | 400 (Regular), 500 (Medium) | system-ui, -apple-system, sans-serif |
| **Code / Monospace** | JetBrains Mono | 400, 500 | ui-monospace, "Cascadia Code", "Fira Code", monospace |
| **Marketing / Hero** | Inter | 800 (Extrabold) | system-ui, sans-serif |

### 4.2 Type Scale (Dashboard)

| Element | Size | Weight | Line Height | Letter Spacing |
|---------|------|--------|-------------|----------------|
| h1 (page title) | 24px / 1.5rem | 700 | 1.2 | -0.025em |
| h2 (section) | 20px / 1.25rem | 600 | 1.3 | -0.02em |
| h3 (card title) | 16px / 1rem | 600 | 1.4 | -0.01em |
| Body | 14px / 0.875rem | 400 | 1.5 | 0 |
| Body small | 13px / 0.8125rem | 400 | 1.5 | 0 |
| Caption | 12px / 0.75rem | 500 | 1.4 | 0.01em |
| Badge | 11px / 0.6875rem | 600 | 1 | 0.02em |
| Code | 13px / 0.8125rem | 400 | 1.6 | 0 |

### 4.3 Type Scale (Marketing / Landing)

| Element | Size | Weight |
|---------|------|--------|
| Hero headline | 56px / 3.5rem | 800 |
| Hero sub | 20px / 1.25rem | 400 |
| Section headline | 36px / 2.25rem | 700 |
| Section sub | 18px / 1.125rem | 400 |
| Feature title | 20px / 1.25rem | 600 |
| Feature body | 16px / 1rem | 400 |

---

## 5. ICONOGRAPHY

### 5.1 Icon System

**Primary icon set**: Lucide React (tree-shakable, consistent stroke width)

| Category | Icons Used |
|----------|-----------|
| Navigation | `LayoutDashboard`, `Box`, `Globe`, `Database`, `HardDrive`, `Key`, `ShoppingBag`, `GitBranch`, `Server`, `Network`, `Settings`, `Users` |
| Actions | `Plus`, `Play`, `Square`, `RotateCw`, `Trash2`, `Upload`, `Download`, `Copy`, `ExternalLink`, `Search` |
| Status | `CheckCircle2`, `XCircle`, `AlertTriangle`, `Clock`, `Loader2`, `Zap` |
| Topology | `Globe2`, `Radio`, `Container`, `Cylinder`, `Box`, `Link2` |
| Providers | Custom SVG icons for: GitHub, GitLab, Bitbucket, Docker, Hetzner, DigitalOcean, Vultr, Cloudflare |

### 5.2 Icon Style Guidelines

- Stroke width: 1.5px (Lucide default)
- Size in UI: 16px (inline), 20px (sidebar nav), 24px (page headers), 48px (empty states)
- Color: inherits text color (`currentColor`)
- No filled icons — stroke-only for consistency

---

## 6. UI COMPONENT STYLE

### 6.1 Card Style

```
Dark mode card:
  background: hsl(224 40% 8%)      — #0F1524
  border: 1px solid hsl(215 25% 18%) — #252F3F
  border-radius: 8px (0.5rem)
  padding: 24px
  shadow: none (flat design)

Light mode card:
  background: #FFFFFF
  border: 1px solid #E2E8F0
  border-radius: 8px
  padding: 24px
  shadow: 0 1px 3px rgba(0,0,0,0.05)
```

### 6.2 Button Styles

| Variant | Dark Mode | Light Mode |
|---------|-----------|------------|
| Primary | `bg-green-500 hover:bg-green-600 text-white` | `bg-green-600 hover:bg-green-700 text-white` |
| Secondary | `bg-secondary hover:bg-secondary/80 text-secondary-foreground` | Same |
| Destructive | `bg-red-500 hover:bg-red-600 text-white` | Same |
| Ghost | `hover:bg-muted text-foreground` | Same |
| Outline | `border border-border hover:bg-muted` | Same |
| Premium | `bg-purple-500 hover:bg-purple-600 text-white` | Same (for upgrade prompts) |

All buttons: `border-radius: 6px`, `height: 36px` (default), `padding: 0 16px`, `font-weight: 500`

### 6.3 Input Style

```
Default:
  height: 36px
  border-radius: 6px
  border: 1px solid var(--border)
  background: var(--background)
  padding: 0 12px
  font-size: 14px

Focus:
  border-color: var(--ring) — Monster Green
  ring: 2px var(--ring) with 50% opacity
  outline: none
```

### 6.4 Sidebar Style

```
Width: 256px (expanded), 64px (collapsed)
Background: var(--sidebar)
Border right: 1px solid var(--border)

Nav item:
  height: 36px
  border-radius: 6px
  padding: 0 12px
  font-size: 14px
  font-weight: 500
  color: var(--sidebar-foreground)
  
Nav item (active):
  background: var(--primary) with 10% opacity
  color: var(--primary)
  font-weight: 600

Nav item (hover):
  background: var(--muted)

Section label:
  font-size: 11px
  font-weight: 600
  text-transform: uppercase
  letter-spacing: 0.05em
  color: var(--muted-foreground)
  padding: 8px 12px
```

### 6.5 Table Style

```
Header row:
  background: var(--muted) with 50% opacity
  font-size: 12px
  font-weight: 600
  text-transform: uppercase
  letter-spacing: 0.05em
  color: var(--muted-foreground)

Body row:
  border-bottom: 1px solid var(--border)
  font-size: 14px
  padding: 12px 16px

Row hover:
  background: var(--muted) with 30% opacity

Selected row:
  background: var(--primary) with 5% opacity
  border-left: 2px solid var(--primary)
```

---

## 7. MARKETING ASSETS

### 7.1 Landing Page Structure

```
┌─────────────────────────────────────────────────────┐
│  Navbar: Logo | Features | Pricing | Docs | GitHub  │
│                                    [Get Started] ★   │
├─────────────────────────────────────────────────────┤
│                                                       │
│  Hero Section (dark bg, centered)                     │
│  ─────────────────────────────────                    │
│  🐲 [Monster Icon — large, animated glow]             │
│                                                       │
│  "Tame Your Deployments"                              │
│  (h1, 56px, extrabold, white)                        │
│                                                       │
│  "Self-hosted PaaS that transforms any VPS into       │
│   a full deployment platform. Single binary.          │
│   Zero config. Production-ready in 60 seconds."       │
│  (body, 20px, gray-400)                              │
│                                                       │
│  [Get Started — Free] [View on GitHub ★]              │
│  (green primary btn)   (outline btn)                  │
│                                                       │
│  `curl -fsSL https://deploy.monster/install.sh | bash`│
│  (code block, click to copy)                          │
│                                                       │
│  [Dashboard Screenshot — dark mode, topology view]    │
│                                                       │
├─────────────────────────────────────────────────────┤
│                                                       │
│  "Replaces Your Entire Stack" (section)               │
│  ─────────────────────────────────────                │
│  Coolify ✗ → DeployMonster ✓                          │
│  Traefik ✗ → Built-in Ingress ✓                       │
│  Portainer ✗ → Drag & Drop Topology ✓                 │
│  Vault ✗ → Built-in Secret Management ✓               │
│  (animated comparison cards)                          │
│                                                       │
├─────────────────────────────────────────────────────┤
│                                                       │
│  "Everything You Need" (feature grid 3×4)             │
│  ─────────────────────────────────────                │
│  🚀 Git → Deploy    📡 Auto SSL      🎯 Load Balance │
│  🐳 Docker Compose  🔑 Secret Vault  📊 Monitoring   │
│  🏪 150+ Marketplace 🌐 DNS Sync    👥 Team & RBAC  │
│  🖥 VPS Providers   📦 Backup & S3   💰 Billing     │
│                                                       │
├─────────────────────────────────────────────────────┤
│                                                       │
│  "Visual Infrastructure" (topology showcase)          │
│  ─────────────────────────────────────                │
│  [Animated GIF/video of topology drag & drop]         │
│  "Drag. Drop. Deploy. See your entire infrastructure  │
│   at a glance."                                       │
│                                                       │
├─────────────────────────────────────────────────────┤
│                                                       │
│  "150+ One-Click Apps" (marketplace showcase)         │
│  ─────────────────────────────────────                │
│  [Scrolling grid of app icons]                        │
│  WordPress • Ghost • Strapi • n8n • Gitea •           │
│  PostgreSQL • Redis • Ollama • Plausible • ...        │
│                                                       │
├─────────────────────────────────────────────────────┤
│                                                       │
│  "For Hosting Providers" (enterprise section)         │
│  ─────────────────────────────────────                │
│  White-label → Sell as your own brand                 │
│  Reseller system → Multi-tier revenue                 │
│  WHMCS integration → Works with your billing          │
│  [Talk to Sales]                                      │
│                                                       │
├─────────────────────────────────────────────────────┤
│                                                       │
│  Pricing Section                                      │
│  ─────────────                                        │
│  [Community: Free] [Pro: $49/mo] [Enterprise: $299]   │
│                                                       │
├─────────────────────────────────────────────────────┤
│                                                       │
│  "Get Started in 60 Seconds"                          │
│  ─────────────────────────────                        │
│  Step 1: curl install                                 │
│  Step 2: ./deploymonster                              │
│  Step 3: Open browser → deploy your first app         │
│  [Start Now — It's Free]                              │
│                                                       │
├─────────────────────────────────────────────────────┤
│  Footer: Logo | Links | GitHub | Discord | Twitter    │
│  © 2026 ECOSTACK TECHNOLOGY OÜ                       │
└─────────────────────────────────────────────────────┘
```

### 7.2 Social Media Assets

**GitHub README Header:**
- 1280×640 banner image
- Dark background with Monster Green gradient
- Logo + tagline + key features (icons)
- `deploy.monster` URL

**Twitter/X Card:**
- 1200×628 image
- Dashboard screenshot with topology view
- Tagline overlay
- GitHub stars count

**Open Graph (og:image):**
- 1200×630
- Logo + "Self-Hosted PaaS" + feature bullets
- Dark theme

### 7.3 Comparison Marketing

**"DeployMonster vs X" content:**

| vs | Our Advantage | One-liner |
|----|---------------|-----------|
| Coolify | 3-panel UI, topology, billing, single binary | "Coolify is a toy. DeployMonster is a platform." |
| Dokploy | Team management, marketplace, VPS providers | "Dokploy deploys. DeployMonster manages." |
| CapRover | Modern UI, secret vault, enterprise features | "CapRover is 2018. DeployMonster is 2026." |
| Portainer | Full deploy pipeline, not just container viewer | "Portainer shows containers. DeployMonster runs your business." |
| Vercel | Self-hosted, no vendor lock-in, unlimited | "Vercel charges per seat. DeployMonster charges nothing." |
| Heroku | Self-hosted, transparent pricing, full control | "Heroku died. DeployMonster rises." |

---

## 8. FAVICON & APP ICONS

### 8.1 Favicon Set

| File | Size | Usage |
|------|------|-------|
| `favicon.ico` | 16×16 + 32×32 (multi-size ICO) | Browser tab |
| `favicon-16x16.png` | 16×16 | Small browser tab |
| `favicon-32x32.png` | 32×32 | Standard browser tab |
| `apple-touch-icon.png` | 180×180 | iOS home screen |
| `android-chrome-192x192.png` | 192×192 | Android home screen |
| `android-chrome-512x512.png` | 512×512 | Android splash |
| `mstile-150x150.png` | 150×150 | Windows tiles |
| `safari-pinned-tab.svg` | vector | Safari pinned tab (single color) |

### 8.2 PWA Manifest

```json
{
  "name": "DeployMonster",
  "short_name": "Monster",
  "description": "Self-hosted PaaS platform",
  "start_url": "/",
  "display": "standalone",
  "background_color": "#0A0F1E",
  "theme_color": "#22C55E",
  "icons": [
    { "src": "/android-chrome-192x192.png", "sizes": "192x192", "type": "image/png" },
    { "src": "/android-chrome-512x512.png", "sizes": "512x512", "type": "image/png" }
  ]
}
```

---

## 9. SOCIAL & COMMUNITY

### 9.1 Presence

| Platform | Handle | Purpose |
|----------|--------|---------|
| GitHub | `github.com/deploy-monster` | Code, issues, releases |
| Twitter/X | `@deploymonster` | Updates, dev tips, engagement |
| Discord | `discord.gg/deploymonster` | Community, support |
| YouTube | `@deploymonster` | Tutorials, demos |
| Dev.to | `dev.to/deploymonster` | Technical articles |

### 9.2 GitHub Repo Badges

```markdown
[![GitHub Stars](https://img.shields.io/github/stars/deploy-monster/deploy-monster?style=flat&logo=github&color=22C55E)](https://github.com/deploy-monster/deploy-monster)
[![License](https://img.shields.io/badge/license-AGPL--3.0-22C55E)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.23+-22C55E?logo=go)](https://go.dev)
[![Docker](https://img.shields.io/docker/pulls/deploymonster/deploymonster?color=22C55E)](https://hub.docker.com/r/deploymonster/deploymonster)
[![Discord](https://img.shields.io/discord/xxxxx?color=8B5CF6&logo=discord&logoColor=white)](https://discord.gg/deploymonster)
```

---

## 10. MONSTER MASCOT

### 10.1 Character Design Brief

**Name**: "Monty" (the DeployMonster mascot)

**Personality:**
- Friendly but powerful
- Eager to help
- Slightly mischievous
- Always working (holding containers, cables, etc.)

**Visual style:**
- Rounded, approachable body shape (not scary)
- Monster Green as primary color
- Small horns or antenna
- Large, expressive single eye (or two eyes)
- Minimal detail — works as icon at small sizes
- Inspired by: GitHub Octocat, Golang Gopher, Kubernetes whale

**Use cases for mascot:**
- Error pages ("Monty is looking for that page...")
- Empty states ("Monty is waiting for your first deploy!")
- Loading states (Monty animation)
- Stickers, swag, community
- Documentation illustrations
- Social media avatar

### 10.2 Mascot Poses

| Pose | Context |
|------|---------|
| Standing proud | Default, README header |
| Holding a container | Docker/deploy context |
| With server rack | Infrastructure context |
| Sleeping | Maintenance/downtime page |
| Running with rocket | Deployment in progress |
| Wearing hard hat | Build in progress |
| With magnifying glass | Search/discovery context |
| Juggling containers | Multi-deploy context |
| Reading book | Documentation context |
| Party hat | Success/celebration |

---

## 11. IMAGE GENERATION PROMPTS

### 11.1 Logo Prompt (for AI image gen)

```
Minimalist geometric logo icon for "DeployMonster" - a self-hosted deployment platform.
Design a stylized friendly monster head/face that subtly incorporates:
- A shipping container or box shape in the silhouette
- A small upward arrow or rocket element (deploy metaphor)
- Clean, modern, geometric style
- Works at 16x16px favicon size
- Single color: #22C55E (bright green) on dark background #0A0F1E
- No text, icon only
- Flat design, no gradients, no shadows
- Tech/developer aesthetic
Style: modern SaaS logo, vector-ready, Silicon Valley startup aesthetic
```

### 11.2 Dashboard Screenshot Prompt (for mockup)

```
Professional dark-theme web application dashboard for a deployment management platform called "DeployMonster".
Left sidebar with green accent navigation items.
Main content area showing:
- Grid of application cards with green status indicators
- A drag-and-drop topology canvas showing connected services (database, web app, cache)
- Real-time metrics charts
Color scheme: dark navy background (#0A0F1E), green accents (#22C55E), purple highlights (#8B5CF6)
Modern, clean, high-information-density design. 
Style: Vercel/Linear dashboard aesthetic but dark mode.
```

### 11.3 GitHub Banner Prompt

```
Wide banner image (1280x640) for GitHub repository.
Dark background (#0A0F1E) with subtle grid pattern.
Left side: DeployMonster logo (green monster icon) glowing with green (#22C55E) light.
Center-right: Text "DeployMonster" in bold white, below: "Tame Your Deployments" in green.
Bottom: Feature icons in a row: rocket, globe, database, shield, chart
Subtle green-to-purple gradient accent line.
Modern, minimal, developer-focused aesthetic.
```

---

## 12. INFOGRAPHIC PROMPT (Nano Banana 2 Compatible)

```
Professional modern infographic poster for "DeployMonster" - a self-hosted PaaS platform.

LAYOUT: Vertical A3 poster, dark background (#0A0F1E)

HEADER:
- Green monster logo icon (geometric, minimal)
- "DeployMonster" text in bold white
- "Tame Your Deployments" tagline in green (#22C55E)

CONTENT SECTIONS (top to bottom):

1. "WHAT IT DOES" — 3 icon blocks:
   - Git Push → Auto Deploy (rocket icon)
   - Docker Compose → Running Stack (container icon)
   - 150+ Marketplace Apps (grid icon)

2. "ARCHITECTURE" — simplified system diagram:
   Binary → [Ingress | Deploy | Build | Discovery | DNS | Monitoring | Backup]
   → Docker Engine

3. "KEY NUMBERS" — stats in large green text:
   - Single Binary
   - 10 Dependencies
   - 150+ Templates
   - 251 Tasks to v1.0
   
4. "VS COMPETITION" — comparison table:
   DeployMonster ✓ vs Coolify ✗ vs Dokploy ✗
   (for: topology, billing, white-label, secret vault)

5. "DEPLOY IN 60 SECONDS" — 3 steps with terminal mockup:
   curl install → ./deploymonster → deploy first app

FOOTER:
- deploy.monster | github.com/deploy-monster
- "AGPL-3.0 | Free Forever | Enterprise When You Need It"
- ECOSTACK TECHNOLOGY OÜ

STYLE: Dark tech aesthetic, green (#22C55E) and purple (#8B5CF6) accents, 
clean typography (Inter font), generous spacing, modern SaaS marketing feel.
No clutter. Professional. Would look good printed or shared on social media.
```

---

*This branding guide ensures consistent visual identity across all DeployMonster touchpoints — from favicon to marketing site to enterprise white-label customization.*

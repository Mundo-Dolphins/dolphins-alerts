# dolphins-alerts

[![CI](https://github.com/Mundo-Dolphins/dolphins-alerts/actions/workflows/ci.yml/badge.svg)](https://github.com/Mundo-Dolphins/dolphins-alerts/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Librería Go (y binario de ejemplo) con los bloques de las alertas de [Mundo Dolphins](https://mundodolphins.es):

- **Alertas de partidos** de un equipo de la NFL desde ESPN hacia Telegram (`espn`).
- **Injury report**: normaliza, compara y renderiza cambios de lesiones y los publica en Telegram (`injury`). Incluye una fuente basada en datos abiertos de [nflverse](https://github.com/nflverse).
- **Espejo en Bluesky** de un feed RSS de tu propia cuenta (`socialfeed`, `rss`, `bluesky`).
- Envío a Telegram (`telegram`), configuración por entorno (`config`) y orquestación (`app`).

> Proyecto de la comunidad Mundo Dolphins. No está afiliado ni respaldado por la NFL, ESPN, Miami Dolphins,
> X, Telegram ni Bluesky; los nombres se usan solo de forma descriptiva. Ver [NOTICE](NOTICE).

## Qué incluye y qué no

Esta librería usa únicamente:

| Fuente | Cómo |
|---|---|
| Telegram, Bluesky | APIs oficiales |
| nflverse (lesiones, calendario) | Datos abiertos, CC BY 4.0 |
| ESPN (marcadores) | Endpoints JSON públicos **no documentados**: pueden cambiar o dejar de funcionar |
| Feed RSS | El que tú indiques; la librería solo lo lee |

**No incluye** ningún scraper de webs de terceros, ni acceso a redes sociales con cookies o credenciales.
Si necesitas otra fuente de lesiones, implementa `injury.InjuryProvider` (ver
[docs/INJURY_REPORT.md](docs/INJURY_REPORT.md)); cada provider es responsable de tener derecho a usar su fuente.

## Uso como librería

```bash
go get github.com/Mundo-Dolphins/dolphins-alerts@latest
```

```go
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Mundo-Dolphins/dolphins-alerts/app"
	"github.com/Mundo-Dolphins/dolphins-alerts/config"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load(false)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// app.WithInjuryProvider(myProvider) sustituye la fuente de lesiones por defecto (nflverse).
	app.New(cfg).Run(ctx)
}
```

> **Estabilidad:** mientras la versión sea `v0.x`, la API puede cambiar entre versiones menores.
> Fija una versión concreta en tu `go.mod`.

## Binario de ejemplo

```bash
cp .env.example .env     # rellena al menos TELEGRAM_BOT_TOKEN y TELEGRAM_CHANNEL_ID
go run ./cmd/alerts --use-test-channel

# Injury report sin publicar ni escribir estado
go run ./cmd/alerts injury check --dry-run --team MIA --season 2026 --week 1
```

## Configuración

Variables de entorno (o `.env`); ver [.env.example](.env.example):

| Variable | Uso |
|---|---|
| `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHANNEL_ID`, `TELEGRAM_CHANNEL_ID_TEST` | Telegram |
| `BLUESKY_HANDLE`, `BLUESKY_APP_PASSWORD` | Publicación en Bluesky |
| `SOCIAL_FEED_TARGET`, `SOCIAL_FEED_BASE_URL` | Feed RSS a reflejar (alias antiguos `TWITTER_TARGET`, `TWITTER_RSS_BASE_URL`) |
| `ESPN_CACHE_FILE` | Fichero de estado de las alertas de partidos |

El token del bot y las credenciales nunca se escriben en los logs ni en los errores.

## Desarrollo

```bash
make ci        # fmt-check + vet + test + lint
make coverage
```

Los tests no usan red real (servidores `httptest` y fakes).

Documentación: [Injury report](docs/INJURY_REPORT.md) · [Espejo RSS → Bluesky](docs/SOCIAL_FEED.md)

## Seguridad

Ver [SECURITY.md](SECURITY.md).

## Licencia

[MIT](LICENSE). Los datos de nflverse están bajo CC BY 4.0 (ver [NOTICE](NOTICE)).

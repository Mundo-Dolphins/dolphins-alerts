# Injury Report

El monitor de lesiones usa un pipeline determinista, sin IA:

```text
InjuryProvider (FallbackProvider: primera fuente sin error)
  ↓
Report normalizado
  ↓
ReportDiffer
  ↓
TelegramRenderer
  ↓
Telegram
```

## Fuentes

La librería incluye un único provider: **`NFLVerseProvider`**, que lee los datos abiertos de
[nflverse](https://github.com/nflverse/nflverse-data) (licencia **CC-BY 4.0**, ver [NOTICE](../NOTICE)):

- lesiones: `https://github.com/nflverse/nflverse-data/releases/download/injuries/injuries_{temporada}.csv`
- rival: calendario `https://github.com/nflverse/nfldata/raw/master/data/games.csv`, cacheado en memoria por semana

nflverse actualiza los datos una vez al día (hacia las 06:00 UTC), así que pueden ir hasta un día por
detrás de las fuentes oficiales. El mensaje lo indica con "⚠️ Fuente alternativa (nflverse)" y enlaza al CSV.
La fecha del report es el `Last-Modified` del CSV.

### Otras fuentes

`InjuryProvider` es una interfaz de un solo método:

```go
type InjuryProvider interface {
	Fetch(ctx context.Context, request ReportRequest) (Report, error)
}
```

Puedes aportar la tuya con `injury.NewFallbackProvider(...)`, `Monitor.SetProvider(...)` o
`app.WithInjuryProvider(...)`. Los helpers `ReportRequest.WithDefaults`, `NewPlayerReport`,
`NormalizePracticeStatus`, `NormalizeGameStatus` y `SortPlayers` están pensados para eso.

> **Responsabilidad:** cada provider es responsable de tener derecho a usar la fuente que lee
> (términos de uso, `robots.txt`, licencias). Esta librería no incluye ni recomienda ningún
> scraper de webs de terceros.

## Modelo

`internal/injury` trabaja con `Report`, `PlayerReport`, `PracticeStatus` y `GameStatus`. Los estados normalizados son:

- práctica: `DNP`, `LP`, `FP`, `NotListed`, `Unknown`
- designación final: `Out`, `Doubtful`, `Questionable`, `NotListed`, `Unknown`

El provider traduce variantes externas como `Did Not Participate`, `Limited Participation` y `Full Participation` al modelo interno. Si el provider falla o el report parece inválido, el monitor registra el error, no publica y no sobrescribe el último snapshot válido.

## Diff y publicación

El hash de HTML ya no decide publicaciones. El monitor compara snapshots semánticos:

- jugador nuevo
- jugador eliminado
- cambios `DNP → LP`, `LP → FP`, `FP → LP`, `LP → DNP`
- cambio de lesión oficial
- cambio de designación final

La temporada, fase y semana se toman del próximo partido de ESPN para mantener
la misma clave durante todas sus ventanas, incluido el tramo después de
medianoche. Si falta ese dato, se calcula a partir de la fecha del partido.
Si el report trae un rival distinto al próximo de ESPN, se descarta sin
publicar ni guardar (`reason=wrong_opponent`); puede ser el report de la jornada
anterior que todavía no se ha sustituido.

Los polls idénticos no publican, incluso tras reiniciar o pasar del viernes al
sábado dentro de la ventana. La idempotencia se refuerza guardando fingerprints deterministas de eventos publicados en `injury_cache.json`.

## Persistencia

`JSONReportStore` guarda snapshots semánticos y publicaciones:

```json
{
  "version": 2,
  "reports": {
    "2026/REG/week-01/MIA": {
      "season": 2026,
      "seasonType": "REG",
      "week": 1,
      "team": "MIA",
      "source": "nflverse",
      "players": []
    }
  },
  "published": {}
}
```

Si existe un `injury_cache.json` V1, se lee como estado legacy y se migra al formato semántico al siguiente guardado. `lastHash`/MD5 queda obsoleto.

## Scheduler

El monitor obtiene el próximo partido real de Miami con `ESPNScheduleProvider`, con caché en memoria de 6 horas, sin modificar `espn_cache.json`. El día del partido se interpreta en `America/New_York`, para que un Sunday Night que empieza el lunes en España siga contando como domingo.

Solo consulta y publica desde las **22:00 hasta las 02:00 del día siguiente, hora de Madrid** (`Europe/Madrid`, con cambio de horario automático), en estas fechas de la semana del próximo partido. El tramo después de medianoche pertenece a la ventana del día anterior; por ejemplo, el report del viernes se sigue consultando hasta las 02:00 del sábado:

| Partido | Días de injury report |
| --- | --- |
| Domingo | Miércoles, jueves y viernes |
| Lunes | Jueves, viernes y sábado |
| Jueves | Lunes, martes y miércoles |
| Viernes o sábado | Martes, miércoles y jueves |
| Miércoles | Domingo, lunes y martes |

Estos días corresponden al [calendario oficial de la NFL](https://operations.nfl.com/calendar-events/nfl-important-dates). No basta con coincidir en el día de la semana: las fechas deben pertenecer al próximo partido, evitando consultas durante la semana de descanso. Si ESPN falla, no publica; si no hay próximo partido o su día no está contemplado, tampoco consulta lesiones.

Durante la ventana consulta cada 15 minutos. Fuera de ella revisa el calendario cada 6 horas como máximo y adelanta el siguiente despertar a las 22:00 cuando corresponda. La comprobación de ventana ocurre antes de descargar el report o modificar su estado. `--force` permite una publicación manual fuera de ventana y `--dry-run` mantiene el diagnóstico disponible a cualquier hora.

## Mensajes

El injury report se publica solo en Telegram, con mensajes detallados y agrupados. No se publica en Bluesky. Las lesiones se traducen con `translations_es.json`.

### Rival

Un provider puede devolver también la lista del próximo rival. Se guarda la sección del rival en el mismo snapshot semanal (`opponentName` y `opponentPlayers`, campos opcionales; los snapshots antiguos siguen siendo válidos) y ambos equipos se publican en un único mensaje. Si el rival no está disponible, se registra el motivo y se publica solo Miami.

- Cada publicación (primer report, actualización o final) muestra los dos equipos completos.
- Se publica cuando cambia el report de cualquiera de los dos equipos, o cuando aparece el rival por primera vez en el snapshot.
- El report final muestra las designaciones de los dos equipos.
- Sin rival en el report, se mantiene el formato de solo Miami.

Primer report:

```text
🏥 Primer Injury Report de la semana: Dolphins vs Minnesota Vikings

🐬 MIAMI
🔴 No entrenaron:
• Player A — isquiotibial

⚔️ MINNESOTA VIKINGS
🟡 Limitados:
• Player B — rodilla

🔗 Fuente: <URL de la fuente del provider>
```

Actualización (estado actual de los dos equipos):

```text
🏥 Injury Report actualizado: Dolphins vs Minnesota Vikings

🐬 MIAMI
🔴 No entrenaron:
• Player A — isquiotibial

⚔️ MINNESOTA VIKINGS
🟢 Participación completa:
• Player B — rodilla
```

Final:

```text
🏥 Injury Report final: Dolphins vs Minnesota Vikings

🐬 MIAMI
❌ OUT
• Player A — isquiotibial

⚔️ MINNESOTA VIKINGS
❓ QUESTIONABLE
• Player B — rodilla
```

## Dry-run

```bash
go run ./cmd/alerts injury check --dry-run --team MIA --season 2026 --week 1
```

El dry-run hace fetch, normalize, diff y render del mensaje de Telegram. No publica ni altera estado persistido.

## Testing

```bash
go vet ./...
go test ./...
```

Los tests no usan red real.

## Fuera de alcance deliberado

- `SQLiteReportStore`
- soporte completo multi-equipo
- narrativa opcional con LLM

# Espejo RSS → Bluesky

El paquete `socialfeed` lee un feed RSS de cuenta (`{SOCIAL_FEED_BASE_URL}/user/{SOCIAL_FEED_TARGET}`) y
publica en Bluesky las entradas nuevas, en orden cronológico y sin duplicados. Está pensado para
**reflejar tu propia cuenta**.

La librería **solo consume RSS**: no accede a ninguna red social, no usa cookies, credenciales ni
navegadores. De dónde sale el feed (un servicio propio, un generador oficial, el RSS de tu web...) y si
tienes derecho a usarlo es cosa tuya.

## Configuración

```bash
# Cuenta a reflejar, con o sin @
SOCIAL_FEED_TARGET=tu_cuenta
# Base del servicio que sirve el feed (por defecto http://127.0.0.1:8090)
SOCIAL_FEED_BASE_URL=http://127.0.0.1:8090
```

Los nombres antiguos `TWITTER_TARGET` y `TWITTER_RSS_BASE_URL` siguen funcionando como alias;
si ambos están definidos, ganan los nuevos.

## Qué hace

- Limpia etiquetas HTML, decodifica entidades (incluido `&amp;amp;`) y elimina URLs cortas `t.co`.
- Guarda los IDs vistos en `last_twitter_id.txt` (permisos `0600`) para no republicar.
- Distingue errores de servicio, HTTP, XML inválido, fecha inválida, item inválido y feed vacío.

## Tests

```bash
go test ./socialfeed ./rss -v
```

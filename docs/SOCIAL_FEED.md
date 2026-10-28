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

- Limpia etiquetas HTML y decodifica entidades (incluido `&amp;amp;`). Las URLs se **conservan**.
- Las URLs del texto se publican como enlaces pulsables (*facets* de Bluesky). Si un recorte por longitud
  caería dentro de una URL, se descarta la URL entera en vez de dejarla rota.
- Las imágenes se toman de los `<enclosure type="image/*">` del item (máximo 4, límite de Bluesky). Los
  vídeos no se copian.
- Para que el feed traiga URLs reales (no `t.co`) e imágenes, el generador debe expandir los enlaces y emitir
  los `<enclosure>`; `twikit-rss` (`rss_builder.py`) lo hace. Con un feed antiguo, las `t.co` se publican tal
  cual (siguen funcionando como enlace) y no hay imágenes.
- Guarda los IDs vistos en `last_twitter_id.txt` (permisos `0600`) para no republicar.
- Distingue errores de servicio, HTTP, XML inválido, fecha inválida, item inválido y feed vacío.

## Tests

```bash
go test ./socialfeed ./rss -v
```

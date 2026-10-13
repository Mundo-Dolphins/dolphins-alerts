# Seguridad

## Reportar una vulnerabilidad

Usa **Security > Report a vulnerability** (aviso privado de GitHub) en este repositorio.
No abras un issue publico con detalles de una vulnerabilidad ni pegues tokens en issues o PRs.

## Buenas practicas al usar el bot

- El token del bot y el chat ID van **solo** en `.env` (en `.gitignore`) o en variables de entorno.
- Si un token se filtra, revocalo en @BotFather (`/revoke`) y genera uno nuevo.
- Los ficheros de estado (`espn_cache.json`, `injury_cache.json`, `last_twitter_id.txt`) no contienen credenciales, pero no deberían versionarse.

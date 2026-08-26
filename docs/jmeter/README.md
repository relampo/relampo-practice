# Script de JMeter — RelampoTickets

Script de referencia basado en [`../GUIA-SCRIPTING.md`](../GUIA-SCRIPTING.md).
Cubre el flujo completo: login → crear evento → editar (If-Match) → comprar →
pagar (con la transformación del `relampo_token`) → confirmar → limpiar → logout.

| Archivo | Qué es |
|---|---|
| `RelampoTickets.jmx` | El plan de pruebas |
| `users.csv` | Data pool con los 500 usuarios |

## Cómo ejecutarlo

**Desde la interfaz gráfica:** abrir `RelampoTickets.jmx` con JMeter. Correr desde
esta carpeta para que encuentre `users.csv`.

**Desde la línea de comandos** (siempre parado en esta carpeta):

```bash
# Contra el sitio público
jmeter -n -t RelampoTickets.jmx -l resultados.jtl

# Contra una instancia local
jmeter -n -t RelampoTickets.jmx -Jhost=localhost -Jprotocol=http -Jport=8080 -l resultados.jtl

# Una sola pasada para depurar
jmeter -n -t RelampoTickets.jmx -Jthreads=1 -Jloops=1 -l resultados.jtl
```

### Parámetros

| Propiedad | Default | Qué controla |
|---|---|---|
| `-Jhost` | `practice.relampo.com` | Dominio del servidor |
| `-Jprotocol` | `https` | `http` o `https` |
| `-Jport` | (vacío) | Puerto; para local, `8080` |
| `-Jthreads` | `5` | Usuarios virtuales |
| `-Jrampup` | `30` | Segundos de rampa |
| `-Jloops` | `2` | Iteraciones por usuario |

> El máximo son **5 sesiones concurrentes por IP**. Con más de 5 threads desde una
> misma máquina, la sexta sesión recibe `429 vus_per_node`.

## Qué contiene

**Configuración**
- HTTP Request Defaults (dominio, protocolo, timeouts)
- HTTP Cookie Manager con *Clear cookies each iteration* — cada iteración es una sesión nueva
- HTTP Header Manager global (`Accept`, `Accept-Language`, `Authorization: Bearer ${bearer}`)
- CSV Data Set Config leyendo `users.csv`
- Thread Group parametrizado

**21 samplers** correspondientes a los pasos de la guía, cada uno con sus
extractores.

**Lógica condicional**
- Simple Controllers agrupando cada tramo de la sesión
- Dos If Controllers después del login, sobre la columna `edad` del CSV:
  `edad >= 18` abre el flujo completo, `edad < 18` no hace nada
- Loop Controller en 3 dentro de la rama de mayores, envolviendo todo el flujo:
  ver sus eventos, crear, editar, comprar, pagar y borrar — tres veces
- El logout está fuera de los dos If, así que lo hacen todos

Un mayor de edad hace 51 muestras por iteración (3 × 17); un menor hace 3:
home, login y logout.

**Extractores (17)**

| Variable | Extractor |
|---|---|
| `csrf_token`, `publish_token`, `gateway_request`, `gateway_correlation`, `view_state`, `ticket_id`, `catalog_id` | Regular Expression Extractor sobre el body |
| `event_rev`, `correlation_id`, `state`, `code` | Regular Expression Extractor sobre **Response Headers** |
| `bearer`, `event_id`, `event_rev_nueva`, `reservation_id`, `relampo_token_a` | JSON Extractor |
| `seat_id` | JSON Extractor con **Match No. 0** (ocurrencia aleatoria) |

**JSR223 PreProcessor (Groovy)** en `POST /pay/start`: transforma el
`relampo_token` del valor A al valor B.

**5 assertions** — home contiene `RelampoTickets`, login devuelve `bearer`, la
reserva devuelve `relampoToken`, el pago responde `302`, y el ticket final está
`CONFIRMED`.

**5 Uniform Random Timers** con los think times de la guía. Para depurar sin
esperar: `-Jjmeter.timer.factor=0.05` reduce todos los tiempos al 5 %.

## Tres detalles que importan

**1. Los valores base64 vienen HTML-escapados.** El `request` de la pasarela y el
`view_state` son base64, y en el HTML el carácter `+` aparece como `&#43;`. Si se
envía tal cual, el servidor responde `403`. Por eso el script los pasa por
`${__unescapeHtml(...)}` antes de enviarlos. Es intermitente: solo falla cuando al
azar el valor contiene un `+`.

**2. El `/logout` no debe seguir el redirect.** Responde `302` hacia `/`, y si
JMeter lo sigue, ese `GET /` abre una sesión nueva que queda huérfana y consume un
cupo del límite de 5 por IP. El sampler tiene *Follow Redirects* desactivado a
propósito.

**3. Cada vuelta del loop arranca con `GET /manage`.** El `publish_token` que
pide la creación del evento es de un solo uso y nace ahí. Si el paso queda
fuera del loop, la segunda vuelta crea el evento con un token ya gastado y el
servidor responde `403`. Por eso el grupo *Mis eventos* está adentro.

**4. El Loop Controller anidado va con *Forever* marcado.** Un Loop Controller
dentro de un If Controller se da por terminado después de la primera pasada y no
se reinicia en las iteraciones siguientes del Thread Group: la compra sucedería
solo en la primera iteración. Con *Forever* marcado se reinicia bien, y quien
corta el ciclo es la cuenta de `${tickets}`.

## Verificación

Ejecutado con 10 iteraciones, una por usuario: 414 muestras, **0 errores**.
Los ocho mayores dieron 3 vueltas al flujo — 3 eventos creados, 3 reservas y 3
borrados cada uno, 51 muestras; los dos menores (`user005`, `user010`) hicieron
3 muestras. Los diez cerraron sesión.

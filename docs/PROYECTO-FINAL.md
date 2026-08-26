# Proyecto final — Script de performance sobre RelampoTickets

**Aplicación:** https://practice.relampo.com
**Material de apoyo:** [`GUIA-SCRIPTING.md`](GUIA-SCRIPTING.md)

---

## Objetivo

Construir un script de performance completo sobre RelampoTickets, con todos sus
valores dinámicos correlacionados, datos externos, validaciones y lógica de
negocio.

El script tiene que correr de punta a punta **sin un solo error**, con varios
usuarios virtuales y varias iteraciones.

La herramienta la elegís vos: Relampo, JMeter, k6 o Gatling. Lo que se evalúa es
el script, no la herramienta.

---

## Contexto

RelampoTickets es un sitio de venta de entradas. Cada usuario entra, administra
sus propios eventos y compra entradas para ellos.

El sitio tiene una regla de negocio: **la venta de entradas es solo para mayores
de edad**. Los menores de 18 años pueden entrar a su cuenta, pero no participan
de la compra.

Tu script tiene que reflejar esa regla.

---

## Lo que se entrega

| Archivo | Contenido |
|---|---|
| El script | El plan de pruebas completo |
| `usuarios.csv` | El archivo de datos |
| `INFORME.md` | Una página: qué correlacionaste, qué te costó, resultado de la corrida |

---

## Requisito 1 — Datos externos

El script **no puede tener usuarios escritos adentro**. Los lee de un archivo CSV
externo, con tres columnas:

```
usuario,contrasena,edad
user001,Pass001!,19
user002,Pass002!,15
user003,Pass003!,34
```

Los usuarios válidos van de `user001` a `user500`, con contraseñas `Pass001!` a
`Pass500!`. Las edades las asignás vos.

**Armá el archivo con una mezcla:** que haya mayores y menores de 18. Si todos
son mayores, no vas a poder demostrar que la lógica del Requisito 4 funciona.

Cada usuario virtual toma una fila distinta y trabaja con ese usuario durante
toda su sesión.

---

## Requisito 2 — El flujo

Cubrí las **13 acciones** del flujo base de la guía:

| # | Acción | Request principal |
|---|---|---|
| 1 | Abrir la home | `GET /` |
| 2 | Iniciar sesión | `POST /api/auth` |
| 3 | Ver "Mis eventos" | `GET /manage` |
| 4 | Crear un evento | `POST /api/manage/events` |
| 5 | Abrir el evento para editar | `GET /manage/events/{id}/edit` |
| 6 | Guardar cambios | `PUT /api/manage/events/{id}` |
| 7 | Ir a comprar | `GET /events` |
| 8 | Ver el catálogo y los asientos | `GET /api/events/{id}/seats` |
| 9 | Reservar el asiento | `POST /api/reservations` |
| 10 | Pagar | `POST /pay/start` |
| 11 | Pasarela de pago | `/pay/authorize` · `/pay/continue` · `/pay/callback` |
| 12 | Confirmar la compra | `POST /pay/confirm` |
| 13 | Cerrar sesión | `GET /logout` |

Los detalles de cada paso —qué manda, qué devuelve, qué esperar— están en la
sección 2 de la guía.

**Todos los valores dinámicos tienen que estar correlacionados.** Son 18, y
viajan por todos los canales posibles: body HTML, body JSON, headers de
respuesta, header `Location` de un 302, query params, path de la URL y cuerpos de
formulario. La tabla de la sección 3 de la guía los lista con su ubicación y un
extractor sugerido para cada uno.

Uno de ellos, el `relampo_token`, llega con un valor y tiene que enviarse con
otro: hay que transformarlo antes de mandarlo. La sección 5 de la guía explica
cómo.

> Si un valor queda hardcodeado, el script funciona la primera vez y falla la
> segunda. Eso cuenta como error.

---

## Requisito 3 — Think times

**Cada request lleva su think time.** Sin excepción.

Un script sin pausas no simula usuarios: simula una inundación. Los tiempos de
referencia están en cada paso de la guía; ajustalos si querés, pero justificá el
criterio en el informe.

| Paso | Think time de referencia |
|---|---|
| 1 — Abrir la home | 5–8 s |
| 2 — Iniciar sesión | 10–15 s |
| 3 — Ver "Mis eventos" | 4–6 s |
| 4 — Crear un evento | 15–25 s |
| 5 — Abrir para editar | 3–5 s |
| 6 — Guardar cambios | 10–15 s |
| 7 — Ir a comprar | 3–5 s |
| 8 — Catálogo y asientos | 5–8 s |
| 9 — Reservar | 3–5 s — **nunca más de 60 s** |
| 10 — Pagar | 2–3 s |
| 11 — Pasarela | 0 s entre los tres, 5–8 s después |
| 12 — Confirmar | 3–5 s |
| 13 — Cerrar sesión | 2–3 s |

Usá tiempos con variación —un rango, no un número fijo— para que los usuarios
virtuales no marchen sincronizados.

**Cuidado con el paso 9:** el token de pago vence a los 60 segundos. Si ponés un
think time largo ahí, el pago responde 403.

---

## Requisito 4 — La lógica de negocio

Este es el punto central del proyecto.

El script tiene que comportarse distinto según la edad del usuario que le tocó
del CSV:

**Si tiene 18 años o más** — completa el recorrido entero: crea su evento, lo
edita, elige asiento, reserva, paga y confirma.

**Si tiene menos de 18** — entra a su cuenta y cierra sesión. **No toca ninguno
de los requests de compra.** Ni el catálogo, ni los asientos, ni la reserva, ni
la pasarela de pago. Esos requests no deben aparecer en sus resultados.

**Los dos cierran sesión.** Nadie se queda con la sesión abierta, sea cual sea su
edad.

No te digo qué elemento usar. Cada herramienta lo resuelve a su manera, y parte
del ejercicio es que encuentres cuál. Lo que se evalúa es que la regla se cumpla
y que los resultados lo demuestren.

> **Cómo se prueba:** filtrá los resultados por usuario. Un menor no puede tener
> ni una sola muestra de `/api/reservations` ni de `/pay/*`. Si aparece aunque
> sea una, la lógica no está funcionando.

---

## Requisito 5 — Tres assertions

Poné **tres validaciones** en puntos distintos del flujo. Elegilas vos, pero que
sirvan: una assertion que solo revisa que el status sea 200 no detecta casi nada,
porque esta aplicación devuelve 200 con mensajes de error adentro.

Tres lugares que valen la pena:

- **El login** — que la respuesta traiga el `bearer`. Si no lo trae, todo lo que
  sigue va a fallar y no vas a saber por qué.
- **La reserva** — que la respuesta traiga el `relampoToken`. Es el valor que hay
  que transformar; si no llegó, el pago falla más adelante.
- **El ticket final** — que el estado sea `CONFIRMED`. Es la prueba de que la
  compra se completó de verdad.

Justificá tu elección en el informe.

---

## Requisito 6 — Organización del flujo

Agrupá el script en bloques con nombre, uno por tramo de la sesión: el ingreso,
la administración de eventos, la compra, el cierre.

En JMeter eso se hace con **Simple Controllers**; en k6 y Gatling con `group`;
en Relampo con los grupos del YAML.

No cambia el comportamiento del script —los bloques no agregan ni sacan
requests—, pero cambia todo lo demás: el reporte queda legible, los tiempos se
leen por tramo, y cualquiera que abra tu script entiende el recorrido sin leer
request por request.

Un script de 20 samplers en una lista plana es ilegible. Ese también es un
criterio de evaluación.

---

## Requisito 7 — Cookies

La aplicación entrega **una sola cookie**, en la respuesta del `GET /` inicial.
Esa cookie identifica la sesión y tiene que viajar en todos los requests
siguientes.

Usá el manejo automático de cookies de tu herramienta —el *cookie manager*—, no
la correlaciones a mano.

**Importante:** configuralo para que **borre las cookies en cada iteración**. Si
no, la segunda iteración reutiliza la sesión de la primera y el escenario deja de
ser realista.

---

## Cómo se corre

| Parámetro | Valor |
|---|---|
| Usuarios virtuales | 5 |
| Iteraciones | 2 o más |
| Rampa | 30 s |

**El límite es de 5 sesiones concurrentes por IP.** El sexto usuario virtual
recibe `429 vus_per_node`. Si querés correr con más carga, tenés que levantar la
aplicación localmente.

Dos detalles que hacen fallar scripts que parecían correctos:

**El logout no debe seguir el redirect.** Responde 302 hacia `/`, y ese `GET /`
abre una sesión nueva que queda huérfana y consume un cupo del límite.

**Algunos valores en base64 vienen HTML-escapados.** El `+` aparece como `&#43;`
dentro del HTML. Si lo mandás así, el servidor responde 403. Es intermitente:
solo falla cuando el valor generado al azar contiene un `+`, así que puede pasar
diez corridas sin aparecer. La sección 2 de la guía, paso 11, explica cómo
revertirlo en cada herramienta.

---

## Criterios de evaluación

| Criterio | Peso |
|---|---|
| El script corre con 0 errores, 5 usuarios × 2 iteraciones | 25 % |
| Los 18 valores dinámicos están correlacionados, ninguno hardcodeado | 25 % |
| La lógica de edad se cumple y los resultados lo demuestran | 20 % |
| Todos los requests tienen think time, con variación | 10 % |
| Las tres assertions son pertinentes y están justificadas | 10 % |
| El flujo está organizado en bloques con nombre | 5 % |
| Cookies manejadas por el manager, limpiadas por iteración | 5 % |

---

## Checklist antes de entregar

- [ ] Los usuarios salen del CSV; no hay ninguno escrito en el script
- [ ] El CSV tiene mayores y menores de edad
- [ ] Corrí 5 × 2 y terminó con 0 errores
- [ ] Ningún menor tiene muestras de `/api/reservations` ni de `/pay/*`
- [ ] Todos los usuarios, de cualquier edad, hacen el `GET /logout`
- [ ] El logout no sigue el redirect
- [ ] Corrí dos veces seguidas y dio lo mismo: nada quedó hardcodeado
- [ ] Cada request tiene su think time
- [ ] Las tres assertions están puestas y justificadas en el informe
- [ ] El script está agrupado en bloques con nombre
- [ ] El cookie manager borra cookies en cada iteración
- [ ] `INFORME.md` está escrito

---

## Para ir más lejos

Opcionales. No suman puntos, pero son los problemas que aparecen en un proyecto
real:

**Comprar varias entradas.** Que cada mayor de edad repita el flujo de compra
tres veces en la misma sesión. Ojo: hay valores de un solo uso que hay que volver
a pedir en cada vuelta. Descubrir cuáles es parte del ejercicio.

**Errores que no son errores.** Esta aplicación devuelve 200 con un cuerpo de
error adentro. ¿Cuántas de tus muestras "exitosas" lo son de verdad?

**Depurar con pistas.** Mandá el header `X-Practice-Hints: true` y los errores te
van a decir exactamente de dónde extraer el valor que falló. Sacalo de la versión
final.

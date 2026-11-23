# RouDash

RouDash muestra cómo llega una petición desde un host hasta un servicio de
Kubernetes. Lee los Ingresses del clúster y presenta sus rutas, namespaces y
plugins de Kong en un panel y un grafo interactivo. Este repositorio contiene
el código fuente, la configuración de despliegue y las pruebas del proyecto.

## Arranque con Docker Compose

Desde la raíz del proyecto, con Docker y el plugin Docker Compose:

```sh
docker compose up -d --build --wait
docker compose ps
curl -fsS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/healthz
```

Abre **<http://127.0.0.1:8080>**. Un `200` en `/healthz` indica que RouDash
está listo, incluso antes de conectar un clúster. Compose construye la imagen
local con el [`Dockerfile`](Dockerfile) y levanta el servicio `roudash`.

| Operación | Comando |
|---|---|
| Ver la configuración efectiva | `docker compose config` |
| Seguir los logs | `docker compose logs -f roudash` |
| Aplicar cambios de código o configuración | `docker compose up -d --build --wait` |
| Detener y retirar el contenedor | `docker compose down` |

## Configuración

Los valores predeterminados están en [`compose.yml`](compose.yml). Para
personalizarlos, copia [`.env.example`](.env.example) a `.env`, edita las
variables y vuelve a ejecutar `docker compose up -d --wait`. No hace falta
crear `.env` para el primer arranque.

| Variable de Compose | Por defecto | Función |
|---|---|---|
| `ROUDASH_PORT` | `8080` | Puerto del navegador en el host. El contenedor escucha en `8080`. |
| `ROUDASH_BIND_ADDR` | `0.0.0.0` | Dirección de publicación: todas las interfaces IPv4. Usa `127.0.0.1` para acceso solo local. |
| `ROUDASH_API_OBFUSCATION` | `true` | Con `true` las rutas de la API rotan cada 30 s; con `false` son estables. |
| `ROUDASH_PERSIST_DIR` | vacío | En blanco usa memoria; `/data` habilita el guardado en el volumen `data`. |

Por ejemplo, con `ROUDASH_PORT=19084` la interfaz y `/healthz` estarán en
`http://127.0.0.1:19084`. RouDash no autentica a los usuarios del panel: la
rotación de rutas no es un control de acceso. Si trabajas con credenciales de
un clúster desde tu portátil, configura `ROUDASH_BIND_ADDR=127.0.0.1` antes
del arranque para limitar el acceso a tu máquina.

Si Docker indica `port is already allocated`, elige un puerto libre en `.env`
y vuelve a levantar el servicio. `docker compose port roudash 8080` muestra
la dirección y el puerto publicados en esa instalación.

### Conectar un clúster

1. Pulsa **Upload Kubeconfig**, selecciona un YAML o JSON de hasta 1 MiB y
   conecta. Si hay varios contextos, elige el que quieras explorar.
2. La API Kubernetes debe ser accesible **desde el contenedor**. Si el
   kubeconfig apunta a `127.0.0.1`, esa dirección normalmente es el propio
   contenedor. Para un túnel local en Linux, sigue la [guía de K3s](demo/guided-demo.md).
3. La identidad necesita permiso `list` sobre `ingresses.networking.k8s.io`
   en todos los namespaces. Incluye las credenciales y certificados en el
   archivo: no se ejecutan plugins `exec` ni se leen certificados del host.

En Compose, RouDash no toma automáticamente `~/.kube/config`. La opción
**In-Cluster SA** es para ejecutar el binario dentro de un pod con cuenta de
servicio, no para conectar Docker al contexto de `kubectl`.

### Guardar kubeconfigs entre reinicios

Por defecto, los kubeconfigs se pierden al reiniciar el proceso. Para conservar
uno, configura `ROUDASH_PERSIST_DIR=/data`, aplica el cambio y marca **Save
kubeconfig to disk** al cargarlo. RouDash restaura los archivos guardados al
arrancar. Los archivos contienen credenciales: limita el acceso al volumen.
`docker compose down` conserva el volumen; añadir `-v` **borra sus datos**.

## Explorar el panel

- **Tema:** pulsa el sol para pasar al tema claro o la luna para volver al oscuro.
  El botón está en la pantalla de conexión y en la barra superior. El navegador
  recuerda tu elección; la primera visita usa el tema oscuro.
- **Workspace filters:** Check namespaces, hosts, or Kong plugins in the sidebar.
  An Ingress can match any checked value within a category and must match every
  category you use. An empty category includes all values. Use the search box
  to narrow the results further.
- **Active filters:** The row below the search box shows what applies to the
  dashboard, its counters, and Graph View. Click a chip’s **×** to remove only
  that filter, or **Clear all** to reset the selections and search. Use the
  document icon beside a sidebar option to inspect it without changing filters.
- **Graph View:** selecciona un nodo para resaltar sus vecinos y abrir sus
  detalles. Arrastra para desplazarte y usa la rueda para acercarte. **Fit
  view** centra el mapa; **Overview only** muestra solo clúster, namespaces e
  Ingresses. Los servicios se deducen de los backends referenciados.
- **Manifiesto:** al seleccionar un elemento, el panel de detalles se abre en
  **YAML**. Pulsa **JSON** para ver el mismo objeto en ese formato. Para hosts,
  clúster y contadores, el selector permite elegir entre los Ingresses asociados.
  Para ver un Namespace, Service o plugin de Kong se necesita permiso `get` sobre
  ese recurso; si no está disponible, el panel muestra el Ingress que lo referencia.
- **Refresh / Auto 30s:** ambos comparten la consulta en curso. Si falla una
  actualización, el panel conserva los últimos datos válidos de ese contexto
  y muestra su hora. Una actualización normal conserva el filtro y el zoom.

## Desarrollo y pruebas

El backend Go sirve la interfaz y sus recursos desde un solo binario. Los
handlers HTTP están en [`handlers/`](handlers/); el cliente Kubernetes, en
[`k8s/`](k8s/); y el almacenamiento opcional, en [`persist/`](persist/).
Los estilos, los iconos y vis-network 10.1.5 se sirven localmente desde
[`handlers/assets/`](handlers/assets/); las licencias del proveedor se incluyen
en `vendor/` dentro de ese directorio.

Para desarrollo nativo con Go 1.26 o posterior:

```sh
go test ./...
go vet ./...
PORT=18085 go run .
```

El binario lee `PORT` (por defecto `8082` fuera de Docker), `LISTEN_ADDR`
(`127.0.0.1`), `API_OBFUSCATION` (`true`) y `PERSIST_DIR` (vacío).

Para la prueba de navegador, deja corriendo esa instancia **nativa**, recién
iniciada y sin persistencia. En otra terminal con Python, Playwright y su
Chromium instalado, ejecuta:

```sh
ROUDASH_TEST_URL=http://127.0.0.1:18085 python3 tests/browser_smoke.py
```

La prueba conecta dos API Kubernetes simuladas en loopback y verifica el grafo,
la renovación de rutas, el cambio de contexto y la navegación móvil.
# roudash

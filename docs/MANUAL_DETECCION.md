# Detección de imágenes: manual de uso y configuración

Este manual explica **qué hace cada ajuste**, qué valor poner según lo que quieras conseguir, cómo leer la tarjeta del
canal y qué hacer cuando algo falla. Para **instalar el detector y entrenar el modelo** está la
[guía paso a paso](GUIA_DETECCION.md) (secciones 2 a 5); aquí se da por hecho que ya tienes el detector funcionando.

## 0. Cómo funciona, en 30 segundos

```
canal en directo ──► imagen (miniatura del canal, o un fotograma de lo que grabas)
                       │  cada «Check every» segundos
                       ▼
                 detector (programa aparte, en tu PC o en Colab) + modelo entrenado
                       │  «hay X con un 87 %»
                       ▼
           ¿coincide con lo que buscas?  (etiquetas + confianza mínima)
              │ sí                              │ no
              ▼                                 ▼
   • marca un tramo de la grabación      • la tarjeta dice «Detect: no match»
   • (opcional) abre la «compuerta»:
     el canal se graba solo mientras coincide
   • (opcional) al acabar, corta un clip por tramo
```

- **Es opcional y está apagado por defecto.** Un canal que no usa detección no gasta nada.
- Hay **dos tipos de modelo**: un *clasificador* de imagen completa («esto es en_vivo / otra_escena») y un detector de
  *cajas* (YOLOX: «persona 87 %, taza 64 %»). Cuál te conviene está explicado en la sección 1 de la guía.
- El detector es un programa separado (`detector/`). GoondVR solo le manda imágenes y lee la respuesta; **si el
  detector se apaga, GoondVR sigue funcionando** y la tarjeta lo dice.

## 1. Lista de comprobación antes de empezar

1. El detector está encendido (la ventana de `detector\start.bat` abierta) y dice `detector on http://127.0.0.1:8099 models: [...]`.
2. Hay al menos un modelo en `detector\models\<nombre>\` (si no tienes uno propio, `yolox_nano_coco` viene listo: 80
   objetos comunes como `person`, `cell phone`, `laptop`, `cup`, `tv`…).
3. **Ajustes → Image detection → Test connection** responde «Connected. Models: …».
4. Si vas a cortar clips o a mirar «la grabación» necesitas **ffmpeg en el PATH** (`ffmpeg -version` en una consola).

## 2. Ajustes globales: *Ajustes → pestaña Image detection*

Estos ajustes valen para **todos** los canales (dónde está el detector). Qué mirar y cuándo se hace en cada canal (sección 3).

| Ajuste | Qué es | Valor recomendado |
|---|---|---|
| **Where the detector runs** | «On this computer»: el detector corre en tu PC y, si hay tarjeta gráfica que compense, la usa sola. «On Google Colab»: corre en un cuaderno de Colab y **las imágenes salen de tu PC**. | **On this computer.** Colab solo para probar (condiciones en la guía, sección 7). |
| **Address on this computer** | Dónde escucha el detector. Vacío = `http://127.0.0.1:8099`. | Déjalo vacío. Cámbialo solo si arrancaste el detector con otro `--port` (p. ej. `http://127.0.0.1:8100`). |
| **Address of the Colab notebook** | La dirección `https://….trycloudflare.com` que imprime el cuaderno. **Debe empezar por `https://`** (con `http://` no se acepta porque el token viajaría sin cifrar). Cambia cada vez que reinicias Colab. | Solo con Colab. |
| **Token** | Contraseña que el detector exige. En local puedes dejarlo vacío (el detector solo escucha en tu PC); en Colab es **obligatorio** (lo imprime el cuaderno). | Vacío en local. |
| **Test connection** | Pregunta al detector qué modelos tiene, sin guardar nada. Si va bien: `Connected. Models: nombre (tipo, CPU/GPU)`. | Úsalo siempre tras cambiar algo. |

Para guardar, el botón *Save* de abajo del todo (vale para todas las pestañas a la vez; no hace falta guardar en cada una).

> El token se guarda en el archivo de configuración de GoondVR en claro, igual que las cookies. No compartas ese archivo.

## 3. Ajustes de cada canal: *rueda del canal → pestaña Image detection*

Se activa canal a canal. Por defecto todos los valores vienen vacíos (= valor por defecto de la tabla).

| Ajuste | Qué hace | Rango (por defecto) | Cuándo cambiarlo |
|---|---|---|---|
| **Look at this channel's pictures with the detector** | Enciende la detección en este canal. | apagado | Solo en los canales que te interesan. |
| **Model** | Carpeta del modelo en `detector\models\`. Al escribir aparece la lista de lo que el detector tiene. | — (obligatorio al encender) | Elige el modelo entrenado para lo que buscas. |
| **Picture from** | **The preview picture**: la miniatura que publica el sitio (ligera; funciona aunque no estés grabando). **The recording**: un fotograma de lo que se está grabando (necesita ffmpeg y que el canal se esté grabando). | preview | Usa *recording* solo si la miniatura no basta (es pequeña o va retrasada). No se puede combinar con «Record only while it matches». |
| **Check every** | Segundos entre dos imágenes del canal. | 2–3600 (10) | Más bajo = más preciso y más trabajo. Con muchos canales sube a 20–30 (sección 6). |
| **It counts when it finds** | Etiquetas que cuentan como «coincide», separadas por comas (hasta 30). Vacío = cualquier cosa que el modelo reconozca. | vacío | Pon solo lo que buscas (p. ej. `en_vivo`, o `person, cell phone`). Con `yolox_nano_coco` **no lo dejes vacío**: casi cualquier imagen contiene «algo». |
| **With at least** | Confianza mínima para aceptar un acierto. | 1–100 % (60) | Falsas alarmas → súbelo (70–80). Se le escapan cosas → bájalo (40–50). Cambia de 5 en 5. |
| **Record only while it matches** | La «compuerta»: el canal se graba **solo mientras hay coincidencia**. | apagado | Para ahorrar disco cuando solo te interesa un tipo de escena. Necesita *preview picture*. |
| **…sec without a match and it stops** | Con la compuerta: tras tantos segundos sin coincidencia, deja de grabar y espera la siguiente. | 5–86400 (60) | Más alto = no corta por un parpadeo del modelo; más bajo = graba menos de más. Pon al menos 3× «Check every». |
| **Cut clips of the matches when the recording is finished** | Al terminar cada grabación corta un clip por cada tramo con coincidencia. | apagado | Si quieres los momentos sueltos sin revisar horas de vídeo. |
| **sec before / sec after** | Segundos extra antes del primer acierto y después del último. | 0–300 (5 y 10) | Súbelos (p. ej. 10 y 15) si los clips empiezan tarde: ver «precisión» abajo. |

**Reglas que el programa comprueba al guardar** (te avisa con un mensaje): modelo obligatorio si está encendido, hasta
30 etiquetas, confianza 1–100 %, intervalo 2–3600 s, espera 5–86400 s, márgenes 0–300 s, y la compuerta exige la
miniatura (no se puede con «The recording»).

### Precisión de los tramos y los clips

El programa mira **una imagen cada «Check every» segundos**, así que el principio de un tramo se marca como mucho con
ese retraso, y con «The recording» (solo se examina el último fotograma clave) algo más: del orden de «Check every»
más lo que dure un trozo de vídeo. Los clips se cortan **sin recodificar** y empiezan en el fotograma clave anterior
(pueden perder hasta ~2 s). Por eso existen «sec before/after». En una prueba con un directo simulado (objeto en
pantalla de 12 s a 26 s, mirando cada 2 s) el tramo salió de 14 s a 26 s: el principio se marcó con ~2 s de retraso.

## 4. Recetas por caso de uso

**A. «Quiero los momentos importantes sin ver horas de vídeo» (grabar todo y sacar clips)**
- Detección encendida · Picture from: *preview* · Check every: **10** · Etiquetas: lo que buscas · With at least: **60**
- *Cut clips* **sí** · before **8** · after **12** · *Record only while it matches* **no**
- Resultado: la grabación completa de siempre + `…_clip01.mp4`, `…_clip02.mp4` y `….detect.json`.

**B. «Solo me interesa grabar cuando aparece X» (ahorrar disco)**
- Detección encendida · Picture from: *preview* · Check every: **5–10** · Etiquetas: lo que buscas · With at least: **65–70**
- *Record only while it matches* **sí** · «without a match…» **60–120** · clips: opcional
- Cuidado: **mientras no hay coincidencia no se graba nada**, así que lo que ocurra entre una imagen y la siguiente se
  puede perder. Si el detector se apaga, la grabación se detiene pasado ese tiempo (sección 7).

**C. «Vigilo muchos canales y mi PC va justo»**
- Check every: **30** (o más) en los canales poco importantes · modelo ligero (clasificador o `yolox_nano_coco`)
- Detector con `--threads 2` (valor por defecto) y *Picture from: preview* (no arranca ffmpeg para cada imagen).
- Activa la detección **solo** en los canales que de verdad la necesitan.

**D. «Quiero la máxima precisión»**
- Check every: **2–5** · With at least: ajústalo mirando la tarjeta unos minutos · modelo entrenado con **tus** imágenes
  (de varios canales, con los casos que fallan) · márgenes de clip altos (10–15 s).

**E. «Probar con el modelo que ya viene (`yolox_nano_coco`)»**
- Model: `yolox_nano_coco` · Etiquetas: `person` (o `cell phone`, `laptop`…) · With at least: **50** · Check every: **10**
- Sirve para ver cómo se comporta todo antes de entrenar nada.

## 5. Cómo leer la tarjeta del canal

Cuando la detección está encendida, la tarjeta tiene un botón de detección (al pulsarlo se abren los ajustes del canal):

| Lo que ves | Significa |
|---|---|
| **Detect: no match** | La última imagen se miró bien y no coincide con lo que buscas. |
| **Detect: <etiqueta>** | La última imagen coincide (p. ej. `Detect: en_vivo`). El texto al pasar el ratón da más detalle. |
| **Detect: error** | No se pudo mirar la imagen; pasa el ratón para ver el motivo (tabla de abajo). Reintenta solo, esperando cada vez más (hasta 5 minutos). |
| *(sin botón)* | La detección no está encendida en ese canal. |

Un canal **no se mira** mientras está fuera del aire; con *Picture from: recording* tampoco mientras no se esté grabando.

### Mensajes de error frecuentes

| Mensaje | Qué hacer |
|---|---|
| `the detector does not answer at … (is it running?)` | Arranca `detector\start.bat` o revisa el puerto (Ajustes → Address on this computer). |
| `the detector refused the token` | El token de Ajustes no coincide con el del detector (en Colab cambia en cada ejecución). |
| `the detector address is empty` / `… is not valid` | Ajustes → Image detection: la dirección debe empezar por `http://` o `https://`. |
| `detector: <mensaje>` | Lo dice el propio detector: suele ser un **modelo que no existe** (comprueba el nombre) o una imagen que no pudo leer. |
| `the site has not given a preview picture yet` | El sitio aún no ha publicado la miniatura del canal; se reintenta solo. |
| `ffmpeg is needed … was not found in PATH` | Instala ffmpeg y que esté en el PATH (hace falta para clips y para «The recording»). |
| `no recorded segment to look at yet` | Con «The recording»: aún no hay vídeo grabado que mirar. No es un fallo. |

En el registro de GoondVR el aviso del detector sale **como mucho una vez por minuto** (no una por canal).

## 6. Rendimiento: cuánto cuesta

- **Sin detección: coste cero.** El motor revisa cada segundo los canales que la usan; los demás cuestan una lectura rápida.
- **Con detección** cada imagen son: una descarga pequeña de la miniatura (decenas de KB), una petición al detector y su respuesta.
  Un máximo de **3 imágenes a la vez** en toda la aplicación, aunque haya cientos de canales.
- **Medido** (CPU de un entorno de pruebas de 2 núcleos, no tu PC; detector con 2 hilos, el valor por defecto):
  `yolox_nano_coco` tarda **~15 ms por imagen** (mediana de 20; máximo 25 ms) con una miniatura de 360×270.
- **Estimado (no medido)** con eso: 100 canales con detección a «Check every» 10 s son ~10 imágenes/s ≈ 0,15 s de CPU
  por segundo. En la práctica **el límite suele ser el sitio, no tu PC**: son ~10 descargas de miniatura por segundo.
  Para muchos canales sube «Check every» a 20–30 s.
- «The recording» cuesta más: ffmpeg saca un fotograma de cada imagen. Úsalo solo en pocos canales.
- Con tarjeta gráfica el detector la usa solo si da lo mismo que la CPU y es al menos un 20 % más rápida (guía, sección 8).
  Para modelos tan pequeños la CPU suele bastar.

## 7. Qué ocurre en los casos raros

- **El detector se apaga y tienes «Record only while it matches»**: sin coincidencias la compuerta se cierra y la
  grabación se detiene pasado «without a match». Si prefieres que grabe siempre, no actives esa casilla.
- **Cierras GoondVR mientras graba**: la grabación y la lista de tramos se guardan; un clip que se estuviera cortando
  en ese instante puede quedarse sin hacer.
- **Más de 50 tramos en una grabación**: se cortan 50 clips y el resto se avisa en el registro. Los tramos que se
  tocan (con sus márgenes) se unen en un solo clip. Un clip dura al menos 1 s.
- **La grabación se divide** (por tamaño o duración): cada archivo tiene su propia lista de tramos y sus propios clips.
- **Cambias de modelo en el detector** (reentrenas): se recarga solo en la siguiente imagen, sin reiniciar nada.

## 8. Qué archivos aparecen

Junto a cada grabación, con el mismo nombre:

```
videos/completed/ana_2026-11-27_20-15-00.mp4              la grabación
videos/completed/ana_2026-11-27_20-15-00.mp4.detect.json  los tramos con coincidencia
videos/completed/ana_2026-11-27_20-15-00_clip01.mp4       un clip por tramo (si «Cut clips»)
```

`….detect.json` es texto legible. `start`/`end` son segundos desde el principio de la grabación:

```json
{ "pre": 5, "post": 10, "clips": true,
  "spans": [ { "start": 12.0, "end": 26.0, "label": "cuadrado", "score": 0.97 } ] }
```

## 9. Qué se probó y qué no

Probado aquí: validación de ajustes, el motor (tiempos, reintentos, compuerta), tramos y clips con ffmpeg, la cadena
completa (sitio simulado → GoondVR real → detector real → tramos y clips), el detector con un clasificador, con cajas
(YOLOX) y con el modelo COCO, y la interfaz en un navegador real (Chromium).

**No probado aquí:** `start.bat` en Windows, tarjeta gráfica real (DirectML/CUDA), los cuadernos de Colab dentro de
Colab y el entrenamiento YOLOX en GPU. Detalles y cómo comprobarlos en tu equipo: guía, sección 11.

# Detección de imágenes: guía paso a paso

Con la detección activada, GoondVR mira una imagen de cada canal cada pocos segundos y le pregunta a un programa
pequeño (el **detector**) qué hay en ella. Con la respuesta puede:

- **grabar solo mientras coincide** lo que buscas («Record only while it matches»), y/o
- **cortar clips** de los momentos en que coincidió, cuando termina la grabación.

Todo es opcional y está **apagado en todos los canales** hasta que lo enciendas en la rueda de ajustes del canal.
Un canal sin detección no gasta nada.

## 1. Elige cómo vas a enseñarle (dos tipos de modelo)

| | **Clasificador** (`embed`) | **Cajas** (`yolox`) |
|---|---|---|
| Qué responde | «esta imagen es *A*, *B* o *C*» (toda la imagen) | «aquí hay un *A* (caja), aquí un *B*» |
| Qué necesitas | carpetas de imágenes, una por clase (con 20-30 por clase ya funciona) | imágenes con una caja dibujada alrededor de cada cosa (cientos, mejor) |
| Dónde se entrena | **en tu PC, sin GPU, en minutos** | **en Google Colab** (con GPU gratis), unos minutos u horas |
| Cuándo conviene | escenas completas: «en vivo», «pantalla negra», «sala vacía» | cosas pequeñas o varias a la vez dentro de la imagen |

Empieza por el **clasificador**: es lo más rápido y lo más ligero. Pasa a cajas solo si el clasificador no basta.

**¿Quieres probar todo ya, sin entrenar nada?** El detector descarga junto con el modelo base `yolox_nano_coco`
(Apache-2.0), un detector de cajas ya hecho para 80 cosas cotidianas con los nombres de COCO, en inglés: `person`,
`cup`, `laptop`, `cell phone`, `bottle`, `tv`, `book`… Pon en el canal Model = `yolox_nano_coco` y, en «It counts when
it finds», por ejemplo `person`. (En una foto de prueba con una persona respondió `person` 93 %, en unos 20 ms de CPU.)

## 2. Instalar el detector en tu PC

Necesitas **Python 3.9 - 3.12** (python.org; en Windows marca «Add Python to PATH»). Después:

- **Windows:** doble clic en `detector\start.bat`
- **Linux / macOS:** `./detector/start.sh`

La primera vez crea un entorno privado (`detector/.venv`), instala lo que necesita y descarga el modelo base
(MobileNetV2, licencia Apache-2.0); tarda unos minutos. Las veces siguientes arranca al instante. Deja la ventana
abierta: mientras esté abierta, el detector funciona. Para comprobarlo abre `http://127.0.0.1:8099/health` en el
navegador: debe responder `{"ok":true}`.

Opciones (se escriben después de `start.bat` / `./start.sh`):

| Opción | Para qué |
|---|---|
| `--device auto` (por defecto) | elige solo entre CPU y tarjeta gráfica (ver §8) |
| `--device cpu` / `--device gpu` | forzar una de las dos |
| `--threads 2` (por defecto) | hilos de CPU por modelo; sube si va justo, baja si lo quieres más ligero |
| `--parallel 2` (por defecto) | imágenes que analiza a la vez |
| `--port 8100` | otro puerto (acuérdate de ponerlo también en GoondVR) |

## 3. Conectarlo con GoondVR

En **Ajustes → Image detection**:

1. «Where the detector runs»: **On this computer** (o «On Google Colab», ver §7).
2. «Address on this computer»: `http://127.0.0.1:8099` (déjalo así si no cambiaste el puerto).
3. Pulsa **Test connection**: debe decir que responde y listar los modelos que encuentra.

El token solo hace falta para Colab (en tu PC el detector solo escucha en tu propio equipo).

## 4. Opción A: entrenar un clasificador (sin GPU)

1. **Reúne imágenes** del mismo tipo que verá el detector. Lo más importante: *que se parezcan a lo que va a ver*.
   Si vas a usar «The preview picture» (la miniatura que GoondVR ya descarga), junta miniaturas; si vas a usar
   «The recording», junta capturas de grabaciones. Para sacar capturas de un vídeo ya grabado (necesitas
   [ffmpeg](https://ffmpeg.org)):

   ```
   ffmpeg -i grabacion.mp4 -vf fps=1/10 capturas/%05d.jpg
   ```

   (una captura cada 10 segundos). Mézclalas de **varios canales y momentos distintos**: si todas vienen de un solo
   canal, el modelo aprende ese canal y no lo que buscas.

2. **Ordénalas en carpetas, una por clase.** El nombre de la carpeta es el nombre de la clase:

   ```
   mis_imagenes/
     en_vivo/          *.jpg
     pantalla_negra/   *.jpg
     otra_escena/      *.jpg
   ```

   Incluye una clase de «todo lo demás» (como `otra_escena`): sin ella el modelo no sabe decir «ninguna de las anteriores».

3. **Entrena** (desde la carpeta `detector`, con el entorno de `start.bat`/`start.sh` ya creado):

   ```
   .venv\Scripts\activate          (Windows)        . .venv/bin/activate          (Linux / macOS)
   python train_classifier.py --data mis_imagenes --name mi_modelo
   ```

   Opcional: `--val-data otras_imagenes` para comprobar con imágenes de **otras transmisiones** (más honesto que
   apartar una parte de las mismas). Al terminar, el programa te dice qué porcentaje acierta y qué clases confunde.
   El resultado queda en `detector/models/mi_modelo/` y el detector lo recoge **sin reiniciar**.

4. **Mejora**: mira en qué falla, añade imágenes de *esos* casos y vuelve a ejecutar el paso 3.

## 5. Opción B: entrenar un detector de cajas (YOLOX, en Colab)

Entrenar cajas necesita una tarjeta gráfica; por eso se hace en Colab (gratis, con GPU T4). El modelo resultante
(YOLOX-Nano, licencia Apache-2.0, unos 4 MB) luego **se usa en tu PC, en CPU, sin problema**.

1. **Reúne imágenes** como en el paso 1 del clasificador. Para cajas hacen falta más: unas **200 o más cajas por
   clase** (con menos de ~50 el resultado es flojo; `prepare_detection.py` te avisa).

2. **Dibuja las cajas** con una herramienta de etiquetado que exporte en **formato YOLO** (un `.txt` por imagen),
   por ejemplo [LabelImg](https://github.com/HumanSignal/labelImg), [CVAT](https://www.cvat.ai) o
   [Label Studio](https://labelstud.io) (gratis, locales) o [Roboflow](https://roboflow.com) (en línea: sube tus
   imágenes a su servicio). Deja el resultado así:

   ```
   mi_dataset/
     images/foto1.jpg ...
     labels/foto1.txt ...      una línea por caja:  clase  centro_x  centro_y  ancho  alto   (todo de 0 a 1)
     classes.txt               un nombre de clase por línea (o un data.yaml con "names:")
   ```

   Las imágenes **sin** ningún `.txt` cuentan como ejemplos de «aquí no hay nada», y eso ayuda contra falsas alarmas:
   incluye algunas.

3. **Conviértelo al formato que usa YOLOX** (desde la carpeta `detector`, con el entorno activado):

   ```
   python prepare_detection.py --data mi_dataset --out mi_dataset_coco
   ```

   Muestra cuántas imágenes de entrenamiento y de validación (15 % por defecto) y cuántas cajas hay por clase.
   Si una línea de un `.txt` está mal, se detiene diciendo el archivo y la línea.

4. **Comprime** la carpeta `mi_dataset_coco` en un `.zip` y **súbelo a tu Google Drive** (carpeta `goondvr`, por
   ejemplo: `Mi unidad/goondvr/mi_dataset_coco.zip`).

5. **Abre el cuaderno** `detector/colab/train_detector.ipynb` en [colab.research.google.com](https://colab.research.google.com)
   (Archivo → Subir cuaderno; o desde GitHub si lo tienes en tu repositorio). Antes de nada:
   *Entorno de ejecución → Cambiar tipo de entorno → GPU (T4)*.

6. **Ejecuta las celdas en orden** (1 a 6). En la celda 1 pon el nombre de tu modelo y la ruta del `.zip` en Drive.
   - Celda 4 (entrenar): es la larga. Las épocas por defecto son 100; con pocas imágenes (menos de ~500) sube a
     200-300. Si Colab se desconecta, vuelve a ejecutar las celdas 1 a 4: continúa donde se quedó (guarda en Drive).
   - Celda 5 comprueba el modelo con **el mismo código que usa el detector** y te dice en cuántas imágenes de
     validación acierta exactamente las clases marcadas.
   - Celda 6 te descarga `mi_detector.zip` (y deja una copia en Drive).

7. **Instala el modelo**: descomprime ese `.zip` dentro de `detector/models/`. Debe quedar
   `detector/models/mi_detector/model.onnx` (junto a `meta.json` y `labels.txt`). El detector lo recoge solo.

## 6. Activarlo en un canal

Rueda de ajustes del canal → **Image detection** → «Look at this channel's pictures with the detector»:

| Ajuste | Qué hace |
|---|---|
| **Model** | nombre de la carpeta en `detector/models/` (la lista aparece al escribir) |
| **Picture from** | «The preview picture»: la miniatura del canal, ligera y funciona aunque no grabes. «The recording»: un fotograma de lo que se está grabando (necesita ffmpeg y que el canal se esté grabando) |
| **Check every** | segundos entre dos imágenes (por defecto 10). Más bajo = más preciso pero más trabajo para tu PC |
| **It counts when it finds** | etiquetas separadas por comas (p. ej. `en_vivo`); vacío = cualquiera que conozca el modelo |
| **With at least** | confianza mínima (por defecto 60 %). Si da falsas alarmas, súbela; si se le escapan cosas, bájala |
| **Record only while it matches** | solo graba mientras hay coincidencia; **necesita «The preview picture»** |
| …sec without a match and it stops | tras ese tiempo sin coincidencia deja de grabar (por defecto 60 s) y vuelve al siguiente acierto |
| **Cut clips of the matches…** | al terminar la grabación corta un clip por cada tramo con coincidencia, con unos segundos antes y después (por defecto 5 y 10) |

Los clips se guardan junto a la grabación como `<nombre>_clip01.mp4`, `_clip02.mp4`… y la lista de tramos queda en
`<nombre>.detect.json`. Para cortar clips hace falta **ffmpeg en el PATH**.

La tarjeta del canal muestra qué ve el detector. Un clasificador puede decir «en_vivo» con 93 %; unas cajas dirán, por
ejemplo, «persona (87 %)».

## 7. Opcional: el detector en Google Colab en vez de tu PC

Se hace con `detector/colab/detector_server.ipynb`: arranca el mismo detector en Colab y lo abre con un túnel de
Cloudflare; al final de la celda 4 imprime una **dirección** (`https://….trycloudflare.com`) y un **token**. En
GoondVR: *Ajustes → Image detection → On Google Colab*, pega la dirección y el token, **Test connection**.

**Antes de decidirte, lo que hay que saber (es importante):**

- **No lo recomiendo para uso continuo.** Las preguntas frecuentes de Colab
  ([research.google.com/colaboratory/faq](https://research.google.com/colaboratory/faq.html)) prohíben, entre otros usos,
  «file hosting, media serving, or other web service offerings not related to interactive compute with Colab»,
  «connecting to remote proxies» y, en las sesiones gratuitas, «bypassing the notebook UI to interact primarily via a
  web UI». Un servidor con túnel se parece a eso: **Google puede limitar o cerrar la sesión.** Sirve para probar un rato.
- Las sesiones gratuitas duran como mucho unas 12 horas y se cierran por inactividad. Al cerrarse, la dirección deja
  de funcionar y la siguiente vez será **otra** (y otro token): hay que pegarlos de nuevo en GoondVR.
- **Privacidad:** las imágenes de tus canales **salen de tu PC** hacia Google y Cloudflare. Con el detector local no sale nada.
- El token es al azar en cada ejecución; sin él nadie puede usar tu detector (solo `/health` responde sin token, y
  solo dice que está encendido).
- Con el detector en CPU de tu PC, el clasificador y YOLOX-Nano pesan poco: para la mayoría de usos **no necesitas Colab para detectar**.

## 8. CPU o tarjeta gráfica: se elige solo

Con `--device auto` (por defecto) el detector, al cargar cada modelo, lo **prueba en la CPU y en cada tarjeta
gráfica que encuentre**, y usa la tarjeta **solo si** da los mismos resultados que la CPU y es al menos un 20 % más
rápida. Si no hay tarjeta, no funciona o no compensa, usa la CPU. Lo que eligió se ve en la ventana del detector y en
«Test connection». Hace falta tener instalado el componente adecuado:

- **Windows (NVIDIA, AMD o Intel):** `start.bat` instala `onnxruntime-directml` si puede.
- **NVIDIA con CUDA (Linux/Windows):** instala `onnxruntime-gpu` en el entorno del detector (`pip install onnxruntime-gpu`
  tras activar `detector/.venv`). `start.sh` lo intenta solo si ve `nvidia-smi`.

Sin tarjeta gráfica no pierdes nada: el modelo base y YOLOX-Nano están pensados para ir bien en CPU.

## 9. Límites y cosas que conviene saber

- **Si el detector se apaga o no responde** y tienes activado «Record only while it matches», **la grabación se
  detiene** pasado el tiempo de «without a match» (no hay coincidencias que mantengan abierta la compuerta). Los
  fallos de conexión se reintentan con esperas cada vez más largas. Si prefieres que grabe siempre, no actives esa casilla.
- «Record only while it matches» **necesita «The preview picture»**: con «The recording» no hay grabación de la que sacar la imagen cuando está parado.
- Los tramos y los clips son **aproximados**: el programa solo mira una imagen cada «Check every» segundos y, con
  «The recording», solo el último fotograma clave (que está al principio del último trozo de vídeo). En una prueba con
  un directo simulado (objeto en pantalla de 12 s a 26 s, mirando cada 2 s) el tramo salió de 14 s a 26 s: el principio
  se marca algo tarde, hasta del orden de «Check every» más lo que dure un trozo de vídeo. Por eso existen los márgenes
  «antes» y «después» de los clips (por defecto 5 y 10 s): **súbelos si te pierdes el principio**. Los clips se cortan
  **sin recodificar**, así que empiezan en el fotograma clave anterior: pueden perder hasta ~2 s al principio.
- Si cierras GoondVR **justo mientras corta un clip**, ese clip puede quedarse sin hacer (la grabación y la lista de
  tramos `.detect.json` sí quedan guardadas).
- Un modelo es tan bueno como sus imágenes: pocas imágenes de un solo canal = un modelo que solo funciona en ese canal.

## 10. Problemas frecuentes

| Síntoma | Qué mirar |
|---|---|
| «Test connection» no responde | ¿está abierta la ventana del detector? ¿el puerto es el mismo en `--port` y en el ajuste? |
| `start.bat` se detiene con un error | Python 3.9-3.12 instalado y en el PATH; ejecútalo otra vez para ver el mensaje |
| «the base model … is missing» | ejecuta `python fetch_models.py` (se descarga una vez; necesita internet) |
| El modelo no aparece en la lista | carpeta `detector/models/<nombre>/` con `meta.json` dentro; el nombre solo letras, números, `_`, `.`, `-` |
| Muchas falsas alarmas | sube «With at least»; añade imágenes de *esos* falsos positivos como «otra_escena» o como imágenes sin cajas |
| No detecta lo que debería | baja «With at least»; añade más imágenes de esos casos, de más canales |
| Los clips no aparecen | ¿está `ffmpeg` en el PATH? Mira el registro del canal: dice por qué falló |
| Colab: «No hay GPU» | *Entorno de ejecución → Cambiar tipo de entorno → GPU (T4)*; la cuota gratuita de GPU a veces se agota: prueba más tarde |

## 11. Comprobar que funciona en tu equipo (10 minutos)

Hay tres cosas que no pude probar donde se escribió esto: Windows, la tarjeta gráfica y Colab. Si estas comprobaciones
salen como se indica, quedan verificadas; si no, copia el texto del error o las últimas líneas de la ventana y el fallo
se puede corregir con eso.

**A. `start.bat` en Windows**

1. Doble clic en `detector\start.bat` (la primera vez tarda unos minutos).
2. Al final debe quedar una ventana abierta con líneas como estas:
   `detector on http://127.0.0.1:8099  models: ['yolox_nano_coco']` y
   `model 'yolox_nano_coco' (yolox) loaded in 0.2s on CPUExecutionProvider {…}`.
3. `http://127.0.0.1:8099/health` en el navegador → `{"ok": true}`.
4. GoondVR → Ajustes → Image detection → **Test connection** → «Connected. Models: yolox_nano_coco (yolox, …)».

**B. Tarjeta gráfica**

La línea `model '…' loaded in … on <X> {…}` del paso A dice dónde corre cada modelo; si hay tarjeta, al final lleva los tiempos que midió en cada candidato (y un `error` si alguna no funcionó), y si no hay ninguna, sale `{}`:
`CPUExecutionProvider` = procesador; `DmlExecutionProvider` = tarjeta en Windows (DirectML); `CUDAExecutionProvider` = NVIDIA.
Es normal que con una tarjeta salga igualmente `CPUExecutionProvider`: con `--device auto` solo usa la tarjeta si da los
mismos resultados y es un 20 % más rápida, y para modelos tan pequeños a menudo no lo es. Para ver si la tarjeta funciona,
abre `start.bat --device gpu`: si no hay ninguna utilizable, no se detiene: usa la CPU (el final de la línea sale `{}` o con el `error` de la tarjeta).

**C. Colab**

- `train_detector.ipynb`: la celda 2 imprime `GPU: …` (si no, activa la GPU); la celda 3, cuántas imágenes y cajas
  tiene cada clase; la celda 4, el progreso («epoch: 1/100, iter: …»); la celda 5, `usando …/best_ckpt.pth` y cuántas imágenes
  de validación salen bien; la celda 6 descarga el `.zip`. Prueba primero con **pocas épocas** (por ejemplo 5) para
  ver que todo encaja antes de dejar horas entrenando.
- `detector_server.ipynb`: la celda 4 imprime «Dirección», «Token» y «Comprobación: True». Pégalos en GoondVR y pulsa
  **Test connection**.

## Qué se probó y qué no

**Probado:** el servidor del detector (clasificador y decodificación de YOLOX, token, límites), `prepare_detection.py`,
el entrenador del clasificador, el motor de detección de GoondVR (compuerta, tramos, ajustes) y `start.sh` de principio a
fin en Linux. Además, la cadena de cajas se probó **en CPU con PyTorch 2.15 y YOLOX 0.3.0**, con un conjunto
sintético: conversión con `prepare_detection.py` → la plantilla `Exp` del cuaderno → pesos preentrenados → cargador de
datos real de YOLOX (con mosaico) → 150 pasos de entrenamiento → exportación a ONNX con el código de la celda 5 → el
decodificador del detector. La forma de la salida fue la esperada y las cajas cayeron en su sitio (IoU medio 0,73 con
solo 150 pasos). Eso demuestra que **los formatos encajan**; no mide la calidad de un modelo real.

**No probado** (no se pudo donde se escribió): `tools/train.py` de YOLOX (exige GPU CUDA), los dos cuadernos **dentro de
Colab** (solo se comprobó que su código es válido y que las partes que no dependen de Colab funcionan), `start.bat` en
Windows y el uso de tarjeta gráfica (DirectML/CUDA). Si algo falla en un cuaderno, el error indica la celda; no sigas
con las siguientes.

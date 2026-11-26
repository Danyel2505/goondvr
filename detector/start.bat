@echo off
rem Starts the detector on this computer (Windows). The first time it creates a private Python
rem environment, installs what it needs and downloads the base model. Extra options go to server.py:
rem   start.bat --device cpu          start.bat --port 8100 --threads 4
cd /d "%~dp0"
if not exist .venv ( py -3 -m venv .venv || goto :error )
call .venv\Scripts\activate.bat
if not exist .venv\ready.txt (
  pip install -q numpy Pillow onnx || goto :error
  rem DirectML uses NVIDIA, AMD and Intel cards; if it cannot be installed the CPU one is used
  pip install -q onnxruntime-directml || pip install -q onnxruntime || goto :error
  echo ok> .venv\ready.txt
)
if not exist models\_base\mobilenetv2_features.onnx python fetch_models.py
python server.py %*
goto :eof
:error
echo.
echo Something failed. Do you have Python 3.9 - 3.12 installed (python.org, with "Add Python to PATH" ticked)?
pause

"""Turns images labelled with boxes (YOLO .txt, what LabelImg / Roboflow / CVAT / Label Studio export) into the
folder that YOLOX trains from (COCO layout), splitting it into training and validation images.

    python prepare_detection.py --data mi_dataset --out mi_dataset_coco

mi_dataset/
    images/foto1.jpg ...        (or the images next to their .txt files, without the images/ and labels/ folders)
    labels/foto1.txt ...        one line per box:  clase  centro_x  centro_y  ancho  alto   (all 0..1 of the picture)
    classes.txt                 one class name per line (or a data.yaml with "names:")
A picture without a .txt is kept as an example of "nothing here" (it helps against false alarms).
"""
import argparse
import json
import os
import random
import re
import shutil
import sys

from PIL import Image

EXT = (".jpg", ".jpeg", ".png", ".webp", ".bmp")


def read_classes(data, explicit=None):
    path = explicit or next((p for p in (os.path.join(data, "classes.txt"), os.path.join(data, "obj.names")) if os.path.exists(p)), None)
    if path:
        with open(path, encoding="utf-8") as f:
            names = [l.strip() for l in f if l.strip()]
    else:
        yaml = os.path.join(data, "data.yaml")
        if not os.path.exists(yaml):
            sys.exit(f"no encuentro los nombres de las clases: pon classes.txt (o data.yaml) en {data}, o usa --classes")
        names = parse_yaml_names(open(yaml, encoding="utf-8").read())
    if not names:
        sys.exit("la lista de clases está vacía")
    if len(set(names)) != len(names):
        sys.exit("hay nombres de clase repetidos")
    return names


def parse_yaml_names(text):
    """The `names:` of a data.yaml: `names: [a, b]`, `- a` lines, or `0: a` lines (no YAML library needed)."""
    m = re.search(r"^names:\s*(\[.*\])\s*$", text, re.M)
    if m:
        return [s.strip().strip("'\"") for s in m.group(1)[1:-1].split(",") if s.strip()]
    m = re.search(r"^names:\s*\n((?:[ \t]+.*\n?)+)", text, re.M)
    out = {}
    for i, line in enumerate((m.group(1) if m else "").splitlines()):
        line = line.strip()
        if line.startswith("-"):
            out[i] = line[1:].strip().strip("'\"")
        elif ":" in line:
            k, v = line.split(":", 1)
            if k.strip().isdigit():
                out[int(k)] = v.strip().strip("'\"")
    return [out[k] for k in sorted(out)]


def find_pairs(data):
    """[(image path, label path or None)]"""
    img_dir = os.path.join(data, "images")
    lab_dir = os.path.join(data, "labels")
    if not os.path.isdir(img_dir):
        img_dir, lab_dir = data, data
    pairs = []
    for root, _, files in os.walk(img_dir):
        for f in sorted(files):
            if f.lower().endswith(EXT):
                rel = os.path.relpath(os.path.join(root, f), img_dir)
                lab = os.path.join(lab_dir, os.path.splitext(rel)[0] + ".txt")
                pairs.append((os.path.join(root, f), lab if os.path.exists(lab) else None))
    return sorted(pairs)


def read_boxes(label_path, n_classes, width, height):
    """[(class index, x, y, w, h)] in pixels; stops with the file and line of a mistake."""
    boxes = []
    if not label_path:
        return boxes
    with open(label_path, encoding="utf-8") as f:
        for n, line in enumerate(f, 1):
            parts = line.split()
            if not parts:
                continue
            try:
                if len(parts) != 5:
                    raise ValueError("se esperan 5 números: clase cx cy ancho alto")
                c = int(parts[0])
                cx, cy, w, h = (float(v) for v in parts[1:])
                if not 0 <= c < n_classes:
                    raise ValueError(f"la clase {c} no existe (hay {n_classes})")
                if not all(0 <= v <= 1 for v in (cx, cy, w, h)):
                    raise ValueError("los valores deben estar entre 0 y 1 (¿exportaste en otro formato?)")
            except ValueError as e:
                sys.exit(f"{label_path}, línea {n}: {e}")
            x1, y1 = max(0.0, (cx - w / 2) * width), max(0.0, (cy - h / 2) * height)
            x2, y2 = min(float(width), (cx + w / 2) * width), min(float(height), (cy + h / 2) * height)
            if x2 - x1 >= 1 and y2 - y1 >= 1:
                boxes.append((c, round(x1, 2), round(y1, 2), round(x2 - x1, 2), round(y2 - y1, 2)))
    return boxes


def build(data, out, classes_file=None, val=0.15, seed=0):
    names = read_classes(data, classes_file)
    pairs = find_pairs(data)
    if len(pairs) < 2:
        sys.exit(f"necesito al menos 2 imágenes en {data}")
    order = list(range(len(pairs)))
    random.Random(seed).shuffle(order)
    n_val = min(len(pairs) - 1, max(1, round(len(pairs) * val)))
    val_ids = set(order[:n_val])
    if os.path.exists(out) and os.listdir(out):
        sys.exit(f"{out} ya existe y no está vacío: bórralo o elige otro --out")

    sets = {"train": {"images": [], "annotations": []}, "val": {"images": [], "annotations": []}}
    per_class = [0] * len(names)
    empty = 0
    for idx, (img_path, lab_path) in enumerate(pairs):
        split = "val" if idx in val_ids else "train"
        folder = os.path.join(out, f"{split}2017")
        os.makedirs(folder, exist_ok=True)
        with Image.open(img_path) as im:
            width, height = im.size
        file_name = f"{idx:06d}{os.path.splitext(img_path)[1].lower()}"
        shutil.copyfile(img_path, os.path.join(folder, file_name))
        d = sets[split]
        image_id = len(d["images"]) + 1
        d["images"].append({"id": image_id, "file_name": file_name, "width": width, "height": height})
        boxes = read_boxes(lab_path, len(names), width, height)
        empty += not boxes
        for c, x, y, w, h in boxes:
            per_class[c] += 1
            d["annotations"].append({"id": len(d["annotations"]) + 1, "image_id": image_id, "category_id": c + 1,
                                     "bbox": [x, y, w, h], "area": round(w * h, 2), "iscrowd": 0})
    os.makedirs(os.path.join(out, "annotations"), exist_ok=True)
    cats = [{"id": i + 1, "name": n, "supercategory": "none"} for i, n in enumerate(names)]
    for split, d in sets.items():
        with open(os.path.join(out, "annotations", f"instances_{split}2017.json"), "w", encoding="utf-8") as f:
            json.dump({"images": d["images"], "annotations": d["annotations"], "categories": cats}, f)
    with open(os.path.join(out, "classes.txt"), "w", encoding="utf-8") as f:
        f.write("\n".join(names) + "\n")
    return {"names": names, "train": len(sets["train"]["images"]), "val": len(sets["val"]["images"]),
            "boxes": per_class, "without_boxes": empty}


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--data", required=True, help="carpeta con images/ y labels/ (formato YOLO .txt)")
    ap.add_argument("--out", help="carpeta de salida (por defecto <data>_coco)")
    ap.add_argument("--classes", help="archivo con los nombres de las clases, uno por línea")
    ap.add_argument("--val", type=float, default=0.15, help="parte de las imágenes para validar (0.15 = 15 %%)")
    ap.add_argument("--seed", type=int, default=0)
    a = ap.parse_args()
    r = build(a.data, a.out or a.data.rstrip("/\\") + "_coco", a.classes, a.val, a.seed)
    print(f"{r['train']} imágenes de entrenamiento, {r['val']} de validación ({r['without_boxes']} sin ninguna caja)")
    for n, c in zip(r["names"], r["boxes"]):
        print(f"  {n}: {c} cajas" + ("   <- pocas: con menos de ~50 cajas por clase el resultado será flojo" if c < 50 else ""))


if __name__ == "__main__":
    main()

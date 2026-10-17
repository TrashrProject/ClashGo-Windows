"""One-device-at-a-time Android pilot CLI. Defaults to observation only."""
from __future__ import annotations

import argparse
import os
import sys
from pathlib import Path

from reqngo.android.adb import AdbClient, AdbError
from reqngo.android.runner import ActionRunner, ExecutionBlocked, Journal, Workflow
from reqngo.android.vision import TemplateVision, VisionError


def main(argv=None):
    parser = argparse.ArgumentParser(description="REQ N GO Android pilot (opt-in)")
    parser.add_argument("command", choices=("devices", "inspect", "dry-run", "execute", "resume", "capture"))
    parser.add_argument("--serial", help="Identifiant ADB precis, jamais choisi automatiquement")
    parser.add_argument("--adb", default=os.environ.get("ADB_BINARY", "adb"))
    parser.add_argument("--manifest", help="References de reconnaissance visuelle calibre")
    parser.add_argument("--workflow", help="Etapes pre/post calibre")
    parser.add_argument("--journal", help="Fichier SQLite reserve a ce profil")
    parser.add_argument("--output", help="Capture PNG locale explicite, ne jamais publier")
    args = parser.parse_args(argv)

    if args.command == "devices":
        for device in AdbClient.devices(adb_binary=args.adb):
            print(device)
        return 0
    if not args.serial:
        parser.error("--serial obligatoire")
    client = AdbClient(args.serial, adb_binary=args.adb)
    if args.command == "capture":
        if not args.output or Path(args.output).suffix.lower() != ".png":
            parser.error("--output doit nommer un nouveau fichier .png local")
        image = client.screenshot()
        # Exclusive creation prevents accidental overwriting of private evidence.
        with Path(args.output).open("xb") as dst:
            dst.write(image.png)
        print("Capture locale creee :", args.output, image.width, image.height)
        return 0
    if not args.manifest:
        parser.error("--manifest obligatoire pour la reconnaissance visuelle")
    vision = TemplateVision(args.manifest)
    if args.command == "inspect":
        screenshot = client.screenshot()
        detected = vision.detect(screenshot)
        print("Appareil:", client.serial, "resolution:", screenshot.width, "x", screenshot.height)
        print("Etat:", detected.state, "confiance:", round(detected.confidence, 3))
        print("Jeu lance:", client.game_running(), "au premier plan:", client.game_foreground())
        return 0
    if not args.workflow or not args.journal:
        parser.error("--workflow et --journal obligatoires")
    flow = Workflow(args.workflow, vision.states)
    if args.command == "execute" and os.environ.get("REQNGO_ANDROID_ACTIONS") != "I_ACCEPT_LOCAL_TOUCHES":
        parser.error("Execution refusee. Activer explicitement REQNGO_ANDROID_ACTIONS.")
    journal = Journal(args.journal)
    try:
        runner = ActionRunner(client, vision, flow, journal)
        output = runner.step(dry_run=args.command != "execute")
        print("Execution:", output)
    finally:
        journal.close()
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (AdbError, VisionError, ExecutionBlocked, ValueError, OSError) as exc:
        print("Pilote arrete:", str(exc), file=sys.stderr)
        raise SystemExit(2)

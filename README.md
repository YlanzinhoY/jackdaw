# Jackdaw

**Jackdaw** is a Windows terminal application for installing community packages and resigning save files for **Assassin's Creed Black Flag Resynced**. It combines the Hypervisor update installer, the Voices38 conversion package, and the save-resigning tools in a single keyboard-driven interface.

## Features

- **Interactive TUI** — use a clear keyboard-driven interface instead of entering several commands manually.
- **Hypervisor update installer** — downloads and installs update 1.0.6 directly into the Black Flag Resynced game folder.
- **Voices38 conversion** — installs the HV to Voices package using the same guided workflow.
- **Automatic Steam discovery** — searches the default Steam installation and every library registered in `libraryfolders.vdf`.
- **Manual game-path support** — accepts a full installation path when Steam discovery is unavailable.
- **Memory-efficient downloads** — processes large files in buffered chunks and writes them directly to disk without loading the complete archive into memory.
- **Safe archive handling** — rejects unsafe paths, links, unsupported entries, and duplicate filenames before installing anything.
- **Built-in RAR fallback** — installs packages even when the Windows `tar.exe` version cannot read the RAR file, including password-protected packages.
- **Batched extraction** — extracts and copies large packages in small batches while showing download and installation progress.
- **Voice-pack cleanup** — removes incompatible drivers, Reflex files, `vbs.cmd`, and `denuvOwO` artifacts after the Voices38 installation.
- **Save resigner** — automatically detects the previous save key and resigns Black Flag saves for another Ubisoft account UUID.
- **Automatic UUID detection** — reads the first valid account folder from Ubisoft's `savegames` directory and pre-fills the UUID in the TUI.
- **Automatic Voices save path** — pre-fills `%APPDATA%\Goldberg UplayEmu Saves\66088` when the folder exists.
- **Game settings synchronization** — finds the relevant settings file by its contents and updates its `[Settings]` `UserId` to match the resigned saves.
- **Automatic backups** — preserves original saves and the game settings file before making changes.
- **Editable interface text** — loads labels, prompts, status messages, and the watermark from `texts.json` without requiring a rebuild.
- **Windows paste sanitization** — removes invisible control characters that can appear when paths are pasted into a terminal.

## Requirements

- Windows 10 or Windows 11
- Assassin's Creed Black Flag Resynced installed through Steam
- A stable internet connection for package installation
- Write access to the game and save folders
- No separate archive utility is required; `tar.exe` is used for regular archives when available, and the application includes a RAR fallback for protected or unsupported archives.

## Running the application

Close the game and Steam, then run:

```powershell
.\bin\jackdaw.exe
```

Use the arrow keys to navigate, `Enter` to confirm, `Esc` to go back, and `Ctrl+C` to cancel.

The main menu contains:

1. **Install update 1.0.6 — Hypervisor**
2. **Install Voices38 pack — HV to Voices**
3. **Resign save files**

Every TUI screen includes the `Made by YlanzinhoY` footer.

## Customizing application text

All regular TUI labels, prompts, status messages, installer descriptions, and the watermark are stored in [`texts.json`](texts.json). Edit the value on the right side of any JSON key, then restart the application. Rebuilding is not required.

The application searches for `texts.json` beside the executable, in the parent directory of a `bin` folder, and in the current working directory. A custom location can also be selected with:

```powershell
$env:ACBFR_TEXTS_FILE = "D:\Jackdaw\my-texts.json"
.\bin\jackdaw.exe
```

Keep placeholders such as `%s` and `%d` in formatted messages because the application replaces them with paths, UUIDs, counts, and progress values.

## Update installation

The updater locates the Steam installation automatically or accepts the full game path manually.

Example:

```text
D:\SteamLibrary\steamapps\common\Assassin's Creed Black Flag Resynced
```

Large downloads are processed in buffered chunks and written directly to disk, so the complete archive is never loaded into memory. The archive is validated for unsafe paths and links before its files are extracted and installed in small batches.

The update URL can be overridden without rebuilding:

```powershell
$env:ACBFR_DOWNLOAD_URL = "https://server.example/update.rar"
.\bin\jackdaw.exe
```

## Voice-pack installation

The voice-pack option uses the same automatic Steam detection, download, archive validation, batched extraction, and installation process.

After installation, it removes the following unwanted components from inside the selected game folder:

- `driver_amd` and `driver_intel` folders;
- `reflex.dll`, `reflex.ini`, and `vbs.cmd`;
- any file or folder whose name contains `denuvOwO`, case-insensitively.

The voice-pack URL is configured in `voices.go` and can also be overridden without rebuilding:

```powershell
$env:ACBFR_VOICES_URL = "https://server.example/voices.rar"
.\bin\jackdaw.exe voices
```

## Black Flag Voices save location

The Black Flag Voices build stores its save files in:

```text
%APPDATA%\Goldberg UplayEmu Saves\66088
```

On a standard Windows installation, this expands to:

```text
C:\Users\<WindowsUser>\AppData\Roaming\Goldberg UplayEmu Saves\66088
```

This is the recommended folder for the Black Flag saves you want to use with the Voices build. Copy your `.save` files into this folder, open **Resign save files** in the TUI, and select this exact folder when asked for the save location.

Typical recognized files include:

```text
ACBlackFlag[AutoSave01].save
ACBlackFlag[ManualSave02].save
ACBlackFlag[Options].save
```

### Prepare your saves before resigning

Follow these steps before choosing **Resign save files** in Jackdaw:

1. Start the Black Flag Voices build and play long enough for the game to create its first save file. Then close the game.
2. Open the folder that contains the saves you used with the Hypervisor build.
3. Copy the Hypervisor `.save` files into the Voices save folder:

   ```text
   C:\Users\<WindowsUser>\AppData\Roaming\Goldberg UplayEmu Saves\66088
   ```

4. Run `jackdaw.exe`, select **Resign save files**, and choose that same `Goldberg UplayEmu Saves\66088` folder when the TUI asks for the save location.
5. Confirm the detected Ubisoft UUID or enter the target account UUID, then let the resigner finish before reopening the game.

## Save resigner

The save resigner performs the following workflow:

1. Reads all recognized `.save` files directly inside the selected folder.
2. Looks for the first valid account UUID folder in:

   ```text
   C:\Program Files (x86)\Ubisoft\Ubisoft Game Launcher\savegames
   ```

3. Pre-fills the detected UUID in the TUI so it can be confirmed or edited.
4. Detects each save's previous encryption key automatically from known save data.
5. Creates backups before modifying any save.
6. Resigns every save with the target account UUID.
7. Locates the Black Flag installation through Steam.
8. Finds the game's settings file by its contents and changes its `[Settings]` `UserId` to the same UUID.

Original saves are copied to a sibling `Backup` folder. `Backup\info.txt` records the target UUID and the previous MD5 value for each converted save.

Before the game settings file is changed, the application creates a copy next to it with the `.acbfr.bak` suffix. Other settings such as `Username`, `Email`, `Language`, DLC entries, items, and chunks are preserved.

If Ubisoft or the game uses a nonstandard location, override the automatic paths:

```powershell
$env:ACBFR_SAVEGAMES_ROOT = "D:\Ubisoft\savegames"
$env:ACBFR_GAME_PATH = "D:\SteamLibrary\steamapps\common\Assassin's Creed Black Flag Resynced"
.\bin\jackdaw.exe
```

## Important notes

- Keep the game and Steam closed while installing packages or resigning saves.
- Package installation overwrites matching files in the game folder.
- Never delete the generated backups until the converted saves have been tested in-game.
- If a batch fails after previous batches were installed, run the installer again or verify the game files through Steam.
- This is an independent community project and is not affiliated with Ubisoft, Assassin's Creed, or Steam.

## Development

```powershell
go mod download
go test ./...
go vet ./...
go build -o bin/jackdaw.exe .
```

Automated tests cover Steam discovery, safe archive extraction, voice-pack cleanup, UUID detection, save-key conversion, backups, and `UserId` synchronization.

## License

Distributed under the [Apache License 2.0](LICENSE).

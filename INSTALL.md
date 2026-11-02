# Install evuedit

The easiest way to install evuedit is to download a prebuilt release. This works on 64-bit macOS and Linux, including Apple Silicon and ARM64 Linux, and does not require Go.

## Install the latest release

Open Terminal and run:

```sh
curl -fsSL https://raw.githubusercontent.com/evilprince/evuedit/main/scripts/install-release.sh | sh
```

To install a specific release, pass its tag to the installer:

```sh
curl -fsSL https://raw.githubusercontent.com/evilprince/evuedit/main/scripts/install-release.sh | sh -s -- --version v0.2.0
```

The installer downloads the matching macOS or Linux binary, checks its SHA-256 checksum, installs it to `~/.local/bin/evuedit`, and creates `~/evueditout` for generated files. If FFmpeg or ImageMagick is missing, it offers to install each using your system package manager. Package installation may ask for your administrator password. Use `--with-ffmpeg` or `--with-imagemagick` to install a missing dependency without a prompt; use `--without-ffmpeg` or `--without-imagemagick` to skip its prompt. To install both without prompts:

```sh
curl -fsSL https://raw.githubusercontent.com/evilprince/evuedit/main/scripts/install-release.sh | sh -s -- --with-ffmpeg --with-imagemagick
```

Go is not needed to run a prebuilt release. During installation, the installer detects your shell and adds the install directory to its startup file (`.zshrc`, Bash's startup file, or Fish config). This is safe to run more than once. Open a new terminal window after installation for the updated `PATH` to take effect; the installer cannot change the environment of a terminal that is already open.

Confirm the installation:

```sh
evuedit --help
```

## Install external tools

The evuedit executable is self-contained, but media processing uses these separate programs:

- FFmpeg, including `ffmpeg` and `ffprobe`, for video presets, image filters, and animated videos.
- ImageMagick, with the `magick` command, for thumbnail generation. The release installer can offer to install it or install it directly with `--with-imagemagick`.

The release installer can offer to install FFmpeg and ImageMagick. The dependency scripts are also available from a source checkout.

## Install from source

To build the current source yourself, install Go 1.22 or later and run:

```sh
git clone https://github.com/evilprince/evuedit.git
cd evuedit
./scripts/install.sh
```

This source installer builds the executable locally. Use `./scripts/install-imagemagick.sh` if you want the repository's optional ImageMagick installer.

## Uninstall

Remove the installed executable:

```sh
rm "$HOME/.local/bin/evuedit"
```

The `~/evueditout` folder contains your generated media and is left untouched.

## Release maintainer notes

After merging code to `main`, create and push a version tag to publish a release:

```sh
git tag v0.2.0
git push origin v0.2.0
```

GitHub Actions builds archives for macOS Intel, Apple Silicon, Linux x86_64, and Linux ARM64, creates `SHA256SUMS`, and publishes them to GitHub Releases. Once a release exists, users can install it with the command above. The repository must be public for anyone to download its releases without signing in.

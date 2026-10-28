# Install evuedit

The easiest way to install evuedit is to download a prebuilt release. This works on 64-bit macOS and Linux, including Apple Silicon and ARM64 Linux, and does not require Go.

## Install the latest release

Open Terminal and run:

```sh
curl -fsSL https://raw.githubusercontent.com/evilprince/evuedit/main/scripts/install-release.sh | sh
```

To install a specific release, pass its tag to the installer:

```sh
curl -fsSL https://raw.githubusercontent.com/evilprince/evuedit/main/scripts/install-release.sh | sh -s -- --version v0.1.0
```

The installer downloads the matching macOS or Linux binary, checks its SHA-256 checksum, installs it to `~/.local/bin/evuedit`, and creates `~/evueditout` for generated files. If FFmpeg is missing, the installer offers to install it. Package installation may ask for your administrator password. To install FFmpeg without a prompt, use `--with-ffmpeg`; to skip the prompt, use `--without-ffmpeg`:

```sh
curl -fsSL https://raw.githubusercontent.com/evilprince/evuedit/main/scripts/install-release.sh | sh -s -- --with-ffmpeg
```

Go is not needed to run a prebuilt release. It is needed only if you build evuedit from source. If `~/.local/bin` is not already on your `PATH`, add this line to your shell startup file (`~/.zshrc` on the default macOS shell, or `~/.bashrc` on many Linux systems), then open a new terminal:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

Confirm the installation:

```sh
evuedit --help
```

## Install external tools

The evuedit executable is self-contained, but media processing uses these separate programs:

- FFmpeg, including `ffmpeg` and `ffprobe`, for video presets and image filters.
- ImageMagick, with the `magick` command, for thumbnail generation.

The installer can offer to install FFmpeg. ImageMagick is still installed separately for `evuthumb`; install it with your system package manager, or use `scripts/install-imagemagick.sh` from a source checkout.

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
git tag v0.1.0
git push origin v0.1.0
```

GitHub Actions builds archives for macOS Intel, Apple Silicon, Linux x86_64, and Linux ARM64, creates `SHA256SUMS`, and publishes them to GitHub Releases. Once a release exists, users can install it with the command above. The repository must be public for anyone to download its releases without signing in.

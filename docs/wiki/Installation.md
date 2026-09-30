# Installation

## macOS

```sh
brew install --cask pareshsahoo902/tap/tripline
```

## Windows

With [Scoop](https://scoop.sh):

```powershell
scoop bucket add tripline https://github.com/pareshsahoo902/scoop-bucket
scoop install tripline
```

Or download `tripline_<version>_windows_amd64.zip` from the [releases page](https://github.com/pareshsahoo902/tripline/releases). Unzip it, put `tripline.exe` in a folder on your `PATH`, and open a new terminal.

## Linux

Download the `linux_amd64` or `linux_arm64` tarball from the [releases page](https://github.com/pareshsahoo902/tripline/releases):

```sh
tar -xzf tripline_*_linux_amd64.tar.gz
sudo mv tripline /usr/local/bin/
```

## From source

With Go 1.23 or newer:

```sh
go install github.com/pareshsahoo902/tripline@latest
```

This puts the binary in `$(go env GOPATH)/bin`, so make sure that folder is on your `PATH`.

## Check it works

```sh
tripline version
tripline scan      # summarizes your most recent Claude Code session
```

Next: [[Hook setup|Hook-Setup]].

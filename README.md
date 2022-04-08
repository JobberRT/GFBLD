# GFBLD
Facebook live video downloader in Go

## PreRequirement
### Custom your own configuration
Copy ```config.example.yaml``` to ```config.yaml```, follow the instruction in it to custom it.
### FFmpeg
GFBLD use exec.Command to execute combine command rather than ffmpeg binding package.So make sure your shell/terminal/cmd can call ffmpeg by "ffmpeg" command(You can do this by adding it to PATH or via brew in MacOS)
#### Macos
```brew install ffmpeg```
#### Windows & Linux
```
1.Download ffmpeg from https://www.ffmpeg.org/download.html
2.Add "ffmpeg" command to PATH
```

## Usage
```
git clone https://github.com/jobber2955/GFBLD
cd GFBLD
go mod vendor
go build .
./GFBLD
```

# net-install

## Installation

```sh
github_install.sh net-install
```

From a working copy:

```sh
local_install.sh net-install
```

## Build

```sh
just build
./net-install --version
```

## Release

```sh
just release patch
```

`versions.txt` is bumped, committed and pushed; the tag and the release are
created by the release workflow.

## Build info

```sh
net-install --version
net-install --origin
net-install --buildinfo
```

The program follows the conventions from
<https://github.com/dimkarp93/install/blob/master/CONVENTIONS.md>.

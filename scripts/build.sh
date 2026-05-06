#!/bin/bash
set -euo pipefail

APP_NAME="cd-proxy"
VERSION="${VERSION:-1.0.0}"
BUILD_DIR="build"

usage() {
    echo "Usage: $0 {darwin|windows|all}"
    echo "  darwin  - Build macOS .app bundle + .dmg"
    echo "  windows - Build Windows .exe"
    echo "  all     - Build both"
    exit 1
}

build_darwin() {
    echo "==> Building macOS binary..."
    GOPROXY=https://goproxy.cn,direct GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o "${APP_NAME}" .

    APP_BUNDLE="${BUILD_DIR}/${APP_NAME}.app"
    DMG_NAME="${BUILD_DIR}/${APP_NAME}-${VERSION}.dmg"
    DMG_TMP="${BUILD_DIR}/dmg-tmp"

    echo "==> Creating app bundle..."
    rm -rf "${APP_BUNDLE}" "${DMG_TMP}"
    mkdir -p "${APP_BUNDLE}/Contents/MacOS"
    mkdir -p "${APP_BUNDLE}/Contents/Resources"

    cp "${APP_NAME}" "${APP_BUNDLE}/Contents/MacOS/${APP_NAME}"
    chmod +x "${APP_BUNDLE}/Contents/MacOS/${APP_NAME}"

    if [ -f icns/cd-proxy.icns ]; then
        cp icns/cd-proxy.icns "${APP_BUNDLE}/Contents/Resources/"
    fi

    cat > "${APP_BUNDLE}/Contents/Info.plist" << PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>${APP_NAME}</string>
    <key>CFBundleIdentifier</key>
    <string>com.cdproxy.app</string>
    <key>CFBundleName</key>
    <string>cd-proxy</string>
    <key>CFBundleDisplayName</key>
    <string>cd-proxy</string>
    <key>CFBundleVersion</key>
    <string>${VERSION}</string>
    <key>CFBundleShortVersionString</key>
    <string>${VERSION}</string>
    <key>CFBundleIconFile</key>
    <string>cd-proxy</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>LSMinimumSystemVersion</key>
    <string>13.0</string>
    <key>LSUIElement</key>
    <true/>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
PLIST

    echo "==> Creating DMG..."
    mkdir -p "${DMG_TMP}"
    cp -R "${APP_BUNDLE}" "${DMG_TMP}/"
    ln -s /Applications "${DMG_TMP}/Applications"

    hdiutil create -volname "${APP_NAME}" \
        -srcfolder "${DMG_TMP}" \
        -ov -format UDZO \
        "${DMG_NAME}"

    rm -rf "${DMG_TMP}"
    rm -f "${APP_NAME}"

    echo "==> Done: ${DMG_NAME}"
    du -h "${DMG_NAME}"
}

build_windows() {
    echo "==> Building Windows binary..."
    mkdir -p "${BUILD_DIR}"
    GOPROXY=https://goproxy.cn,direct GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o "${BUILD_DIR}/${APP_NAME}.exe" .

    echo "==> Done: ${BUILD_DIR}/${APP_NAME}.exe"
    du -h "${BUILD_DIR}/${APP_NAME}.exe"
}

case "${1:-}" in
    darwin)  build_darwin ;;
    windows) build_windows ;;
    all)
        build_darwin
        echo
        build_windows
        ;;
    *) usage ;;
esac

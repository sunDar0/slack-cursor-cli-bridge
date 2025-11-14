#!/bin/bash

set -e

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🔨 Slack-Cursor-Hook 크로스 컴파일"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo

# 빌드 디렉토리 정리
BUILD_DIR="dist"
rm -rf $BUILD_DIR
mkdir -p $BUILD_DIR

# 버전 정보
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S')
echo "📦 버전: $VERSION"
echo "🕐 빌드 시간: $BUILD_TIME"
echo

# 빌드 플래그
LDFLAGS="-s -w -X main.Version=$VERSION -X main.BuildTime=$BUILD_TIME"

# 플랫폼 목록
PLATFORMS=(
    "darwin/amd64"   # macOS Intel
    "darwin/arm64"   # macOS Apple Silicon (M1/M2/M3)
    "windows/amd64"  # Windows x86_64
)

echo "🎯 빌드 타겟:"
for platform in "${PLATFORMS[@]}"; do
    echo "   - $platform"
done
echo

# 각 플랫폼별 빌드
for platform in "${PLATFORMS[@]}"; do
    GOOS=$(echo $platform | cut -d'/' -f1)
    GOARCH=$(echo $platform | cut -d'/' -f2)
    
    OUTPUT_NAME="slack-cursor-hook-${GOOS}-${GOARCH}"
    if [ $GOOS = "windows" ]; then
        OUTPUT_NAME+=".exe"
    fi
    
    echo "🔨 빌드 중: $GOOS/$GOARCH"
    
    env GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=1 go build \
        -ldflags "$LDFLAGS" \
        -o "$BUILD_DIR/$OUTPUT_NAME" \
        cmd/server/main.go 2>&1 | grep -v "warning" || true
    
    if [ $? -eq 0 ] && [ -f "$BUILD_DIR/$OUTPUT_NAME" ]; then
        # 실행 권한 부여
        chmod +x "$BUILD_DIR/$OUTPUT_NAME"
        SIZE=$(du -h "$BUILD_DIR/$OUTPUT_NAME" | cut -f1)
        echo "   ✅ 완료: $OUTPUT_NAME ($SIZE)"
    else
        echo "   ⚠️  실패: $OUTPUT_NAME (CGO 필요)"
    fi
    echo
done

# CGO 없이 다시 시도 (SQLite 제외하면 가능)
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🔄 CGO 없이 재시도 (순수 Go 빌드)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo

for platform in "${PLATFORMS[@]}"; do
    GOOS=$(echo $platform | cut -d'/' -f1)
    GOARCH=$(echo $platform | cut -d'/' -f2)
    
    # Windows는 반드시 no-CGO로 빌드
    if [ $GOOS = "windows" ]; then
        OUTPUT_NAME="slack-cursor-hook-${GOOS}-${GOARCH}.exe"
        
        echo "🔨 빌드 중: $GOOS/$GOARCH (no-CGO, 필수)"
        
        env GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=0 go build \
            -ldflags "$LDFLAGS" \
            -o "$BUILD_DIR/$OUTPUT_NAME" \
            cmd/server/main.go 2>&1 | grep -v "warning" || true
        
        if [ $? -eq 0 ] && [ -f "$BUILD_DIR/$OUTPUT_NAME" ]; then
            # 실행 권한 부여
            chmod +x "$BUILD_DIR/$OUTPUT_NAME"
            SIZE=$(du -h "$BUILD_DIR/$OUTPUT_NAME" | cut -f1)
            echo "   ✅ 완료: $OUTPUT_NAME ($SIZE)"
        else
            echo "   ❌ 실패: $OUTPUT_NAME"
        fi
    else
        OUTPUT_NAME="slack-cursor-hook-${GOOS}-${GOARCH}-nocgo"
        
        echo "🔨 빌드 중: $GOOS/$GOARCH (no-CGO)"
        
        env GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=0 go build \
            -ldflags "$LDFLAGS" \
            -o "$BUILD_DIR/$OUTPUT_NAME" \
            cmd/server/main.go 2>&1 | grep -v "warning" || true
        
        if [ $? -eq 0 ] && [ -f "$BUILD_DIR/$OUTPUT_NAME" ]; then
            # 실행 권한 부여
            chmod +x "$BUILD_DIR/$OUTPUT_NAME"
            SIZE=$(du -h "$BUILD_DIR/$OUTPUT_NAME" | cut -f1)
            echo "   ✅ 완료: $OUTPUT_NAME ($SIZE)"
        else
            echo "   ❌ 실패: $OUTPUT_NAME"
        fi
    fi
    echo
done

# 결과 요약
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📊 빌드 결과"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo
ls -lh $BUILD_DIR/
echo

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ 빌드 완료!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo
echo "📦 빌드 파일 위치: $BUILD_DIR/"
echo
echo "💡 사용 방법:"
echo "   각 플랫폼에 맞는 파일을 전달하세요:"
echo "   - macOS Intel:     slack-cursor-hook-darwin-amd64"
echo "   - macOS M1/M2/M3:  slack-cursor-hook-darwin-arm64"
echo "   - Windows:         slack-cursor-hook-windows-amd64.exe"
echo
echo "⚠️  참고: SQLite는 CGO가 필요합니다."
echo "   CGO 빌드가 실패한 경우, no-cgo 버전을 사용하세요."
echo "   (단, SQLite 기능은 제외됩니다)"
echo


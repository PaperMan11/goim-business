#!/usr/bin/env bash
# 注册联调测试账号，并可选校验 IM 开通状态与 IM token 兑换。
# 依赖：bash、curl（无需 jq）。
#
# 用法：
#   bash scripts/register-test-user.sh
#   bash scripts/register-test-user.sh -l alice -p Test12345678 -n "Alice"
#   bash scripts/register-test-user.sh -b http://127.0.0.1:18180 -l bob --skip-im-check
#
# 注意：password 必须同时包含字母与数字，否则服务端返回 code=1001。

set -euo pipefail

BASE_URL="http://127.0.0.1:18180"
LOGIN_NAME=""
PASSWORD="Test12345678"
NICKNAME="联调测试"
DEVICE_ID="web-test-001"
PLATFORM="5"
SKIP_IM_CHECK=0

usage() {
    sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        -b|--base-url)     BASE_URL=$2; shift 2 ;;
        -l|--login-name)   LOGIN_NAME=$2; shift 2 ;;
        -p|--password)     PASSWORD=$2; shift 2 ;;
        -n|--nickname)     NICKNAME=$2; shift 2 ;;
        -d|--device-id)    DEVICE_ID=$2; shift 2 ;;
        -P|--platform)     PLATFORM=$2; shift 2 ;;
        --skip-im-check)   SKIP_IM_CHECK=1; shift ;;
        -h|--help)         usage; exit 0 ;;
        *) echo "未知参数: $1" >&2; usage >&2; exit 2 ;;
    esac
done

if ! command -v curl >/dev/null 2>&1; then
    echo "缺少依赖: curl" >&2
    exit 2
fi

if [[ -z "$LOGIN_NAME" ]]; then
    LOGIN_NAME="test$(date +%m%d%H%M%S)"
fi

BASE_URL=${BASE_URL%/}

# 转义为 JSON 字符串内容；仅支持单行输入。
json_escape() {
    printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'
}

# 提取数字字段，如 "code":0。
json_number() {
    printf '%s' "$1" | grep -oE "\"$2\":-?[0-9]+" | head -n1 | cut -d: -f2
}

# 提取字符串字段，如 "accessToken":"..."。
json_string() {
    printf '%s' "$1" | grep -oE "\"$2\":\"[^\"]*\"" | head -n1 | cut -d: -f2- | sed -e 's/^"//' -e 's/"$//'
}

api() {
    local method=$1 path=$2 token=${3:-} payload=${4:-}
    local args=(-sS -X "$method" -H "X-Device-Id: $DEVICE_ID" -H "X-Client-Platform: $PLATFORM")
    if [[ -n "$token" ]]; then
        args+=(-H "Authorization: Bearer $token")
    fi
    if [[ -n "$payload" ]]; then
        args+=(-H "Content-Type: application/json; charset=utf-8" --data-binary "$payload")
    fi
    curl "${args[@]}" "$BASE_URL$path"
}

echo "注册账号: $LOGIN_NAME -> $BASE_URL/biz-api/v1/account/register"
payload=$(printf '{"loginName":"%s","password":"%s","nickname":"%s"}' \
    "$(json_escape "$LOGIN_NAME")" "$(json_escape "$PASSWORD")" "$(json_escape "$NICKNAME")")

if ! resp=$(api POST /biz-api/v1/account/register "" "$payload"); then
    echo "注册请求失败：无法连接 $BASE_URL" >&2
    exit 2
fi

code=$(json_number "$resp" code)
if [[ "${code:-}" != "0" ]]; then
    echo "注册失败: code=${code:-unknown} msg=$(json_string "$resp" msg)" >&2
    exit 1
fi

user_id=$(json_string "$resp" userId)
access=$(json_string "$resp" accessToken)
echo "注册成功: userId=$user_id loginName=$(json_string "$resp" loginName) nickname=$(json_string "$resp" nickname)"
echo "accessToken=$access"
echo "refreshToken=$(json_string "$resp" refreshToken)"
echo "sessionId=$(json_string "$resp" sessionId) platform=$(json_number "$resp" platform)"

if [[ "$SKIP_IM_CHECK" -eq 1 ]]; then
    exit 0
fi

if ! status_resp=$(api GET /biz-api/v1/im/status "$access"); then
    echo "IM 状态查询失败：请求未完成" >&2
    exit 2
fi

status_code=$(json_number "$status_resp" code)
if [[ "${status_code:-}" != "0" ]]; then
    echo "IM 状态查询失败: code=${status_code:-unknown} msg=$(json_string "$status_resp" msg)（IM 未启用时属于预期）" >&2
    exit 0
fi

im_status=$(json_number "$status_resp" status)
echo "IM 开通状态: status=$im_status syncedAt=$(json_string "$status_resp" syncedAt)"
if [[ "$im_status" != "1" ]]; then
    echo "IM 尚未就绪，可稍后调用 POST /biz-api/v1/im/retry 或等待 cron 补偿" >&2
    exit 0
fi

if ! token_resp=$(api POST /biz-api/v1/im/token "$access"); then
    echo "IM token 兑换失败：请求未完成" >&2
    exit 2
fi

token_code=$(json_number "$token_resp" code)
if [[ "${token_code:-}" != "0" ]]; then
    echo "IM token 兑换失败: code=${token_code:-unknown} msg=$(json_string "$token_resp" msg)" >&2
    exit 0
fi

echo "IM token 兑换成功: platformId=$(json_number "$token_resp" platformId) apiBaseUrl=$(json_string "$token_resp" apiBaseUrl) wsUrl=$(json_string "$token_resp" wsUrl)"
echo "userId=$(json_string "$token_resp" userId) imToken=$(json_string "$token_resp" token)"

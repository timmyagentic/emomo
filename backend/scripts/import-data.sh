#!/bin/sh
# 数据导入脚本（唯一支持的数据导入入口）
# 用于从本地静态图片目录导入数据到数据库和向量库
# 内部使用 go run，无需预先编译

set -e

# 切换到项目根目录（脚本所在目录的上级）
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

# 加载 .env 文件（如果存在）
if [ -f .env ]; then
    export $(grep -v '^#' .env | grep -v '^$' | xargs)
fi

# 默认配置
DEFAULT_CONFIG="${CONFIG_PATH:-./configs/config.yaml}"
DEFAULT_LIMIT=0
DEFAULT_SOURCE=localdir

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 打印带颜色的消息
info() {
    echo "${BLUE}[INFO]${NC} $1"
}

success() {
    echo "${GREEN}[SUCCESS]${NC} $1"
}

warning() {
    echo "${YELLOW}[WARNING]${NC} $1"
}

error() {
    echo "${RED}[ERROR]${NC} $1"
}

limit_label() {
    if [ "$1" -le 0 ] 2>/dev/null; then
        echo "全部"
    else
        echo "$1"
    fi
}

# 显示使用说明
usage() {
    cat << EOF
用法: $0 [选项]

说明:
    本脚本是唯一支持的数据导入入口；请通过 -p/--path 指定本地静态图片目录。

选项:
    -s, --source SOURCE         数据源类型（默认: ${DEFAULT_SOURCE}）
                                - localdir: 从本地静态图片目录导入
    -p, --path PATH             本地静态图片目录，覆盖配置中的 sources.localdir.root_path
    -l, --limit LIMIT           导入数量限制（默认: 0，表示导入目录中的全部图片）
    -e, --embedding NAME        使用单一路 embedding 配置名称（如 qwen3vl_caption）
                                留空则使用默认配置
    --profile NAME              使用多路检索 profile 导入（默认: 配置中的 search.default_profile）
    -c, --config PATH           配置文件路径（默认: ${DEFAULT_CONFIG}）
    -f, --force                 强制重新处理，跳过重复检查
    -m, --auto-migrate          启用数据库 AutoMigrate（默认: 否）
    -r, --retry                 重试 pending 状态的数据
    -h, --help                  显示此帮助信息

示例:
    # 从本地目录导入全部图片
    $0 -p ./data/memes

    # 只抽样导入 50 条数据
    $0 -p ./data/memes -l 50

    # 使用 qwen3vl profile 导入 image + keyword sparse-only 两路向量
    $0 -p ./data/memes --profile qwen3vl

    # 强制重新处理（跳过重复检查）
    $0 -p ./data/memes -f

    # 重试 pending 状态的数据
    $0 -r -l 50

EOF
}

# 检查 Go 环境
check_go() {
    if ! command -v go >/dev/null 2>&1; then
        error "未找到 Go 环境"
        error "请先安装 Go: https://go.dev/doc/install"
        exit 1
    fi
}

# 运行内部 ingest worker（不作为用户入口直接暴露）
run_ingest() {
    EMOMO_IMPORT_DATA_ENTRYPOINT=script go run ./cmd/ingest "$@"
}

# 主函数
main() {
    local source_type="$DEFAULT_SOURCE"
    local source_path=""
    local limit="$DEFAULT_LIMIT"
    local embedding=""
    local profile=""
    local config_path="$DEFAULT_CONFIG"
    local force=false
    local auto_migrate=false
    local retry=false

    # 解析命令行参数
    while [ $# -gt 0 ]; do
        case "$1" in
            -s|--source)
                source_type="$2"
                shift 2
                ;;
            -p|--path)
                source_path="$2"
                shift 2
                ;;
            -l|--limit)
                limit="$2"
                shift 2
                ;;
            -e|--embedding)
                embedding="$2"
                shift 2
                ;;
            --profile)
                profile="$2"
                shift 2
                ;;
            -c|--config)
                config_path="$2"
                shift 2
                ;;
            -f|--force)
                force=true
                shift
                ;;
            -m|--auto-migrate)
                auto_migrate=true
                shift
                ;;
            -r|--retry)
                retry=true
                shift
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                error "未知选项: $1"
                usage
                exit 1
                ;;
        esac
    done

    # 检查配置文件
    if [ ! -f "$config_path" ]; then
        error "配置文件不存在: $config_path"
        exit 1
    fi

    # 检查 Go 环境
    check_go

    # 如果是重试模式
    if [ "$retry" = true ]; then
        info "重试 pending 状态的数据..."
        info "配置文件: $config_path"
        info "限制数量: $(limit_label "$limit")"

        local cmd="internal ingest worker --retry --limit=$limit --config=$config_path"
        if [ -n "$embedding" ]; then
            cmd="$cmd --embedding=$embedding"
        fi
        if [ -n "$profile" ]; then
            cmd="$cmd --profile=$profile"
        fi
        if [ "$auto_migrate" = true ]; then
            cmd="$cmd --auto-migrate"
        fi

        info "执行命令: $cmd"
        run_ingest --retry --limit="$limit" --config="$config_path" \
            $([ -n "$embedding" ] && echo "--embedding=$embedding") \
            $([ -n "$profile" ] && echo "--profile=$profile") \
            $([ "$auto_migrate" = true ] && echo "--auto-migrate")
        exit 0
    fi

    if [ "$source_type" != "localdir" ]; then
        error "不支持的数据源类型: $source_type"
        error "支持的类型: localdir"
        exit 1
    fi

    # 显示导入信息
    echo ""
    info "=========================================="
    info "开始数据导入"
    info "=========================================="
    info "数据源: $source_type"
    if [ -n "$source_path" ]; then
        info "本地目录: $source_path"
    fi
    info "限制数量: $(limit_label "$limit")"
    info "配置文件: $config_path"
    if [ -n "$embedding" ]; then
        info "Embedding 配置: $embedding"
    elif [ -n "$profile" ]; then
        info "Search Profile: $profile"
    else
        info "Search Profile: 默认"
    fi
    if [ "$force" = true ]; then
        info "强制模式: 是（跳过重复检查）"
    fi
    if [ "$auto_migrate" = true ]; then
        info "AutoMigrate: 启用"
    fi
    info "=========================================="
    echo ""

    # 构建参数
    local args="--source=$source_type --limit=$limit --config=$config_path"

    if [ -n "$source_path" ]; then
        args="$args --path=$source_path"
    fi
    if [ -n "$embedding" ]; then
        args="$args --embedding=$embedding"
    fi
    if [ -n "$profile" ]; then
        args="$args --profile=$profile"
    fi
    if [ "$auto_migrate" = true ]; then
        args="$args --auto-migrate"
    fi

    if [ "$force" = true ]; then
        args="$args --force"
    fi

    info "执行内部导入 worker: $args"
    echo ""

    # 执行导入
    run_ingest --source="$source_type" --limit="$limit" --config="$config_path" \
        $([ -n "$source_path" ] && echo "--path=$source_path") \
        $([ -n "$embedding" ] && echo "--embedding=$embedding") \
        $([ -n "$profile" ] && echo "--profile=$profile") \
        $([ "$force" = true ] && echo "--force") \
        $([ "$auto_migrate" = true ] && echo "--auto-migrate")
}

# 运行主函数
main "$@"

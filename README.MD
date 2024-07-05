# 安装

```
go build
mv term-logger /usr/local/bin
mkdir -p ~/.config/term-logger
cp config.json ~/.config/term-logger
```

# 使用

1. 用 macos terminal.app 创建一个终端用来打印日志

```sh
tty # 可能是 /dev/ttys000
```

2. 修改 ~/.config/term-logger 中的 log 为上一步的 /dev/ttys000

3. 劫持 kitty 的标准输入

```sh
kitty term-logger
```

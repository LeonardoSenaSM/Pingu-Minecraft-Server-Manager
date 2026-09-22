# Pingu — Minecraft Server Manager

[Português (Brasil)](#portugues-brasil) • [English](#english)

---

<a id="portugues-brasil"></a>

## Português (Brasil)

Pingu é um aplicativo desktop para Windows que cria e administra servidores Minecraft Java + Bedrock com PaperMC, Geyser e Floodgate. A interface foi desenvolvida em Go com Fyne v2 e não abre uma janela de terminal.

O repositório oficial deve usar o nome **`Pingu-Minecraft-Server-Manager`** e o executável principal é **`pingu.exe`**.

## Download para usuários

Não é necessário instalar Go, GCC ou configurar Java manualmente.

1. Abra a página [Releases](../../releases/latest).
2. Baixe **`Pingu-Setup-VERSAO-Windows-x64.exe`**.
3. Execute o instalador.
4. Escolha o idioma, confirme a pasta de instalação e os atalhos.
5. Marque **Executar o Pingu** ao concluir.

O instalador inclui:

- `pingu.exe`;
- runtime Eclipse Temurin Java 25 LTS de 64 bits;
- atalhos do Menu Iniciar e, opcionalmente, da Área de Trabalho;
- desinstalador padrão do Windows;
- todos os componentes nativos necessários para executar a interface.

Na primeira vez em que o usuário clicar em **PLAY**, o próprio Pingu baixa as versões compatíveis e mais recentes do PaperMC, Geyser, Floodgate, Chunky e CoreProtect. Essa etapa precisa de conexão com a internet.

Também é publicada uma versão portátil, **`Pingu-VERSAO-Windows-x64-Portable.zip`**. Basta extrair todo o ZIP e executar `pingu.exe`; a pasta `runtime` deve permanecer ao lado do executável.

## Java 25 ou superior

O Pingu exige Java **25 ou superior** para iniciar o PaperMC.

- O instalador e o pacote portátil já incluem Java 25 LTS.
- O Pingu procura primeiro `./runtime/bin/java.exe`.
- Se o runtime incorporado não existir, ele procura um Java instalado no `PATH`.
- Java 24 ou anterior é recusado com uma mensagem clara na interface.
- Versões superiores, como Java 26, também são aceitas.

Java 25 foi escolhido como runtime padrão por ser uma versão LTS. Isso oferece uma base mais estável para usuários comuns, sem impedir o uso de versões posteriores.

## Principais recursos

- Cinco abas: início, console, jogadores, controles e configurações/arquivos.
- Cards independentes para os links Java e Bedrock, com endereço, versão e badge ON/OFF.
- Cópia do endereço `IP:porta` diretamente para a área de transferência.
- Proxy Java TCP e Bedrock UDP escutando em `0.0.0.0`.
- Detecção automática de portas livres a partir de `25565/TCP` e `19132/UDP`.
- Inicialização do Paper no primeiro pacote recebido.
- Parada graciosa após 15 minutos sem jogadores ou tráfego.
- Shutdown dos links com cancelamento de contexto, fechamento dos sockets e liberação das portas.
- Download assíncrono de Paper e plugins com progresso e cancelamento.
- Console via stdin, OP, kick, banimento temporário e pardon.
- Português do Brasil e inglês com troca sem reiniciar.
- Buffer circular limitado a 2.500 logs.
- Configurações persistidas em `server/config.json`.

## Estrutura do repositório

```text
Pingu-Minecraft-Server-Manager/
├── .github/workflows/release.yml   # build e publicação automática
├── installer/Pingu.iss             # instalador Inno Setup
├── scripts/build-release.ps1       # build completo e runtime Java
├── internal/
│   ├── api/                         # APIs e downloads atômicos
│   ├── gui/                         # interface Fyne e painel de links
│   ├── i18n/                        # traduções PT-BR/EN
│   ├── manager/                     # Paper, estado e configurações
│   └── proxy/                       # proxies TCP/UDP
├── build.bat                        # build local simplificado
├── FyneApp.toml
├── go.mod
├── go.sum
├── main.go
└── README.md
```

## Compilar localmente no Windows

### Pré-requisitos para desenvolvedores

- Windows 10 ou 11 de 64 bits;
- Go 1.23 ou superior;
- GCC/MinGW-w64 para o CGO usado pelo Fyne;
- Inno Setup 6;
- PowerShell 5.1 ou superior;
- acesso à internet para baixar o Temurin Java 25.

Com Chocolatey aberto como administrador:

```powershell
choco install golang mingw innosetup -y
```

Depois, execute:

```powershell
build.bat 3.0.0
```

Os arquivos finais serão criados em `dist/`:

```text
dist/
├── Pingu-Setup-3.0.0-Windows-x64.exe
├── Pingu-3.0.0-Windows-x64-Portable.zip
└── SHA256SUMS.txt
```

Para gerar somente o executável durante o desenvolvimento:

```powershell
$env:CGO_ENABLED = "1"
go build -buildvcs=false -trimpath -ldflags="-H=windowsgui -s -w" -o pingu.exe .
```

## Testes

```powershell
go mod download
go test -buildvcs=false -tags ci ./...
go vet -buildvcs=false -tags ci ./...
go test -race -buildvcs=false ./internal/manager ./internal/proxy ./internal/api ./internal/i18n
```

## Dados do usuário e desinstalação

O instalador utiliza, por padrão:

```text
%LOCALAPPDATA%\Programs\Pingu
```

A pasta `server` contém mundos, configurações, logs e plugins. O desinstalador não apaga automaticamente esses dados, reduzindo o risco de perder um mundo por engano. Antes de remover a pasta manualmente, faça backup.

## Firewall e acesso externo

Para jogadores da mesma rede, compartilhe os endereços exibidos na aba Início.

Para acesso pela internet:

- redirecione no roteador a porta TCP exibida para Java;
- redirecione a porta UDP exibida para Bedrock;
- permita `pingu.exe` e o Java incorporado no Firewall do Windows.

O Pingu não utiliza Playit.gg ou outro túnel de terceiros.

## Segurança e assinatura do instalador

O instalador gerado é funcional, mas não é assinado digitalmente. Por isso, o Microsoft Defender SmartScreen pode exibir um aviso nas primeiras instalações. Para distribuição pública profissional, assine `pingu.exe` e o instalador com um certificado de assinatura de código antes de publicar a Release.

O workflow gera `SHA256SUMS.txt` e uma atestação de proveniência dos artefatos para facilitar a verificação do download.

## Licença

Antes de tornar o repositório público, defina uma licença para o código-fonte. Sem um arquivo `LICENSE`, o código permanece protegido pelos direitos autorais padrão e terceiros não recebem permissão automática para modificar ou redistribuir o projeto.

## Referências

- [Fyne — documentação oficial](https://docs.fyne.io/)
- [PaperMC — documentação oficial](https://docs.papermc.io/)
- [GeyserMC — documentação oficial](https://geysermc.org/wiki/)
- [Eclipse Temurin — downloads oficiais](https://adoptium.net/temurin/releases/)
- [Inno Setup — site oficial](https://jrsoftware.org/isinfo.php)
- [GitHub Releases — documentação oficial](https://docs.github.com/repositories/releasing-projects-on-github/about-releases)

[Voltar ao topo](#pingu--minecraft-server-manager)

---

<a id="english"></a>

## English

Pingu is a Windows desktop application that creates and manages Minecraft Java + Bedrock servers using PaperMC, Geyser, and Floodgate. Its interface is built with Go and Fyne v2 and runs without opening a terminal window.

The official repository should be named **`Pingu-Minecraft-Server-Manager`**, and the main executable is **`pingu.exe`**.

## Download for users

Users do not need to install Go, GCC, or configure Java manually.

1. Open the [Releases](../../releases/latest) page.
2. Download **`Pingu-Setup-VERSION-Windows-x64.exe`**.
3. Run the installer.
4. Choose the language, confirm the installation directory, and select the desired shortcuts.
5. Select **Launch Pingu** when the installation finishes.

The installer includes:

- `pingu.exe`;
- a 64-bit Eclipse Temurin Java 25 LTS runtime;
- Start Menu and optional Desktop shortcuts;
- a standard Windows uninstaller;
- all native components required to run the graphical interface.

The first time the user clicks **PLAY**, Pingu automatically downloads the latest compatible versions of PaperMC, Geyser, Floodgate, Chunky, and CoreProtect. An internet connection is required for this step.

A portable package named **`Pingu-VERSION-Windows-x64-Portable.zip`** is also published. Extract the complete ZIP and run `pingu.exe`; the `runtime` directory must remain next to the executable.

## Java 25 or newer

Pingu requires Java **25 or newer** to start PaperMC.

- The installer and portable package already include Java 25 LTS.
- Pingu checks `./runtime/bin/java.exe` first.
- If the bundled runtime is unavailable, Pingu looks for Java in the system `PATH`.
- Java 24 or older is rejected with a clear message in the interface.
- Newer versions, such as Java 26, are also supported.

Java 25 is bundled because it is an LTS release. This provides a stable default for regular users without preventing the use of newer Java versions.

## Main features

- Five tabs: Home, Console, Players, Server Controls, and Settings/Files.
- Separate Java and Bedrock connection cards with address, version, and ON/OFF badges.
- One-click copying of the `IP:port` address to the clipboard.
- Java TCP and Bedrock UDP proxies listening on `0.0.0.0`.
- Automatic free-port detection starting at `25565/TCP` and `19132/UDP`.
- PaperMC starts when the first packet is received.
- Graceful shutdown after 15 minutes without players or traffic.
- Link shutdown with context cancellation, socket closure, and immediate port release.
- Asynchronous PaperMC and plugin downloads with progress and cancellation.
- stdin console, OP, kick, temporary ban, and pardon commands.
- Brazilian Portuguese and English with live language switching.
- Circular log buffer limited to 2,500 entries.
- Persistent settings stored in `server/config.json`.

## Repository structure

```text
Pingu-Minecraft-Server-Manager/
├── .github/workflows/release.yml   # automated build and publishing
├── installer/Pingu.iss             # Inno Setup installer
├── scripts/build-release.ps1       # full build and Java runtime
├── internal/
│   ├── api/                         # APIs and atomic downloads
│   ├── gui/                         # Fyne interface and connection panel
│   ├── i18n/                        # PT-BR/EN translations
│   ├── manager/                     # PaperMC, state, and settings
│   └── proxy/                       # TCP/UDP proxies
├── build.bat                        # simplified local build
├── FyneApp.toml
├── go.mod
├── go.sum
├── main.go
└── README.md
```

## Building locally on Windows

### Developer requirements

- 64-bit Windows 10 or 11;
- Go 1.23 or newer;
- GCC/MinGW-w64 for the CGO support required by Fyne;
- Inno Setup 6;
- PowerShell 5.1 or newer;
- internet access to download Eclipse Temurin Java 25.

Using Chocolatey from an Administrator terminal:

```powershell
choco install golang mingw innosetup -y
```

Then run:

```powershell
build.bat 3.0.0
```

The final files will be created in `dist/`:

```text
dist/
├── Pingu-Setup-3.0.0-Windows-x64.exe
├── Pingu-3.0.0-Windows-x64-Portable.zip
└── SHA256SUMS.txt
```

To generate only the executable during development:

```powershell
$env:CGO_ENABLED = "1"
go build -buildvcs=false -trimpath -ldflags="-H=windowsgui -s -w" -o pingu.exe .
```

## Tests

```powershell
go mod download
go test -buildvcs=false -tags ci ./...
go vet -buildvcs=false -tags ci ./...
go test -race -buildvcs=false ./internal/manager ./internal/proxy ./internal/api ./internal/i18n
```

## User data and uninstallation

By default, the installer uses:

```text
%LOCALAPPDATA%\Programs\Pingu
```

The `server` directory contains worlds, settings, logs, and plugins. The uninstaller does not automatically delete this data, reducing the risk of losing a Minecraft world by mistake. Create a backup before removing the directory manually.

## Firewall and external access

For players on the same network, share the addresses displayed on the Home tab.

For internet access:

- forward the displayed TCP port for Java Edition on the router;
- forward the displayed UDP port for Bedrock Edition;
- allow `pingu.exe` and the bundled Java runtime through Windows Firewall.

Pingu does not use Playit.gg or another third-party tunneling service.

## Installer security and code signing

The generated installer is fully functional but is not digitally signed. Microsoft Defender SmartScreen may therefore display a warning during early installations. For professional public distribution, sign both `pingu.exe` and the installer with a code-signing certificate before publishing the Release.

The workflow generates `SHA256SUMS.txt` and build provenance attestations to help users verify downloaded artifacts.

## License

Choose a source-code license before making the repository public. Without a `LICENSE` file, the source remains protected by standard copyright law, and third parties do not automatically receive permission to modify or redistribute it.

## References

- [Fyne — official documentation](https://docs.fyne.io/)
- [PaperMC — official documentation](https://docs.papermc.io/)
- [GeyserMC — official documentation](https://geysermc.org/wiki/)
- [Eclipse Temurin — official downloads](https://adoptium.net/temurin/releases/)
- [Inno Setup — official website](https://jrsoftware.org/isinfo.php)
- [GitHub Releases — official documentation](https://docs.github.com/repositories/releasing-projects-on-github/about-releases)

[Back to top](#pingu--minecraft-server-manager)

# Pingu — Gerenciador de Servidor Minecraft

[English](README.md)

Pingu é um aplicativo desktop nativo para Windows que cria e administra servidores Minecraft Java + Bedrock com PaperMC, Geyser e Floodgate. A interface foi desenvolvida em Go com Fyne v2 e não abre uma janela de terminal.

## Baixar e instalar

O usuário não precisa instalar Go, GCC nem configurar o Java manualmente.

1. Abra a [Release mais recente](../../releases/latest).
2. Baixe `Pingu-Setup-VERSAO-Windows-x64.exe` para a instalação assistida ou `Pingu-Portable-VERSAO-Windows-x64.zip` para o pacote portátil.
3. Execute o instalador e escolha o idioma, a pasta de instalação e os atalhos.
4. Marque **Executar o Pingu** ao concluir.

O instalador inclui o `pingu.exe` vinculado estaticamente, o runtime Eclipse Temurin Java 25 LTS de 64 bits, atalhos do Menu Iniciar, um atalho da Área de Trabalho marcado por padrão e o desinstalador padrão do Windows. São solicitados privilégios de Administrador porque o aplicativo é instalado em Arquivos de Programas de 64 bits.

Na primeira vez em que **PLAY** for selecionado, o Pingu baixa as versões compatíveis mais recentes do PaperMC, Geyser, Floodgate, Chunky e CoreProtect. Essa etapa requer conexão com a internet.

### Microsoft Defender SmartScreen

O instalador não é assinado digitalmente, então o Microsoft Defender SmartScreen pode exibir um aviso. Se o arquivo veio da página oficial de Releases deste repositório e o SHA-256 corresponde ao `SHA256SUMS.txt`:

1. Clique em **Mais informações**.
2. Confira se o nome e a origem do arquivo estão corretos.
3. Clique em **Executar assim mesmo**.

Não ignore o SmartScreen quando o arquivo tiver sido obtido de uma origem desconhecida.

## Requisito do Java

O Pingu exige Java **25 ou superior** para iniciar o PaperMC.

- O instalador inclui o Java 25 LTS, e o Pingu procura primeiro `./runtime/bin/java.exe`.
- Se o runtime incorporado não existir, o Pingu procura um Java instalado no `PATH`.
- Java 24 ou anterior é recusado com uma mensagem clara na interface.
- Versões posteriores, como Java 26, também são aceitas.

## Principais recursos

- Cinco abas: Início, Console, Jogadores, Controles e Configurações/Arquivos.
- Cards separados para Java e Bedrock com endereço, versão compatível e status ON/OFF em tempo real.
- Cópia do endereço `IP:porta` com um clique.
- Proxies Java TCP e Bedrock UDP escutando em `0.0.0.0`.
- Detecção automática de portas livres a partir de `25565/TCP` e `19132/UDP`.
- Inicialização do PaperMC no primeiro pacote recebido.
- Parada graciosa após 15 minutos sem jogadores ou tráfego.
- Encerramento imediato dos proxies com cancelamento de contexto e fechamento dos sockets.
- Downloads assíncronos de dependências com progresso e cancelamento.
- Comandos de console, OP, kick, banimento temporário e pardon.
- Troca instantânea entre português do Brasil e inglês.
- Buffer circular do console limitado a 2.500 entradas.
- Configurações persistidas em `server/config.json`.

## Estrutura do repositório

```text
Pingu-Minecraft-Server-Manager/
├── .github/workflows/release.yml
├── installer/pingu.iss
├── scripts/build-release.ps1
├── internal/
│   ├── api/
│   ├── gui/
│   ├── i18n/
│   ├── manager/
│   └── proxy/
├── FyneApp.toml
├── go.mod
├── go.sum
├── LICENSE
├── main.go
├── README.md
└── README.pt-BR.md
```

## Compilar no Windows

Pré-requisitos para desenvolvedores:

- Windows 10 ou 11 de 64 bits;
- Go 1.23 ou superior;
- GCC/MinGW-w64 para o build CGO do Fyne;
- Inno Setup 6;
- PowerShell 5.1 ou superior;
- acesso à internet para baixar o Eclipse Temurin Java 25.

Instale as ferramentas em um PowerShell aberto como administrador:

```powershell
choco install golang mingw innosetup -y
```

Compile a aplicação, o instalador e o pacote portátil:

```powershell
.\scripts\build-release.ps1 -Version 3.0.0
```

Os arquivos da distribuição serão criados em `dist/`:

```text
dist/
├── Pingu-Setup-3.0.0-Windows-x64.exe
├── Pingu-Portable-3.0.0-Windows-x64.zip
└── SHA256SUMS.txt
```

Para compilar somente o executável durante o desenvolvimento:

```powershell
$env:CGO_ENABLED = "1"
go build -buildvcs=false -trimpath -ldflags="-H=windowsgui -s -w -extldflags '-static'" -o pingu.exe .
```

O script de Release e o workflow também inspecionam o executável com `objdump.exe`. O build falha se `pingu.exe` importar `libmcfgthread-2.dll`, `libgcc_s_seh-1.dll`, `libstdc++-6.dll` ou `libwinpthread-1.dll`.

## Testes

```powershell
go mod download
go test -buildvcs=false -tags ci ./...
go vet -buildvcs=false -tags ci ./...
go test -race -buildvcs=false ./internal/manager ./internal/proxy ./internal/api ./internal/i18n
```

## Dados do usuário e rede

A pasta padrão de instalação é `%ProgramFiles%\Pingu Minecraft Server Manager`. Os dados graváveis do servidor ficam separados em `%LOCALAPPDATA%\Pingu Minecraft Server Manager\server`, que contém mundos, configurações, logs e plugins. O desinstalador preserva esses arquivos para reduzir o risco de perder um mundo acidentalmente; faça backup antes de apagá-los manualmente.

Para jogadores na mesma rede, compartilhe os endereços exibidos na aba Início. Para acesso pela internet, redirecione no roteador as portas TCP do Java e UDP do Bedrock exibidas na tela e permita o `pingu.exe` e o Java incorporado no Firewall do Windows. O Pingu não utiliza Playit.gg nem outro túnel de terceiros.

## Licença

Distribuído sob a [Licença MIT](LICENSE).

## Documentação oficial

- [Fyne](https://docs.fyne.io/)
- [PaperMC](https://docs.papermc.io/)
- [GeyserMC](https://geysermc.org/wiki/)
- [Eclipse Temurin](https://adoptium.net/temurin/releases/)
- [Inno Setup](https://jrsoftware.org/isinfo.php)

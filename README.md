# Anansi

Gerenciador desktop de bancos de dados MySQL e MariaDB, feito com **Wails 2 + Go + Vue 3/TypeScript**.

Inclui as funcionalidades do [HeidiSQL](https://www.heidisql.com/) para MySQL/MariaDB, várias delas
melhoradas — veja o [inventário](docs/heidisql/inventario.md) e o registro das etapas em
[`docs/heidisql/`](docs/heidisql/README.md).

- **Conexões**: perfis salvos com ambiente (produção em vermelho), pastas, comentários, TLS, **túnel SSH
  embutido** (senha, chave ou agente, com verificação da chave do servidor), **socket Unix**, script inicial,
  keep-alive, filtro de databases, exportação/importação sem senhas; senhas no keyring do sistema
- **Árvore e abas de objetos**: servidor (databases, variáveis editáveis, status, processos com KILL e
  EXPLAIN, estatísticas de comandos), database (lista de objetos com tamanhos), tabela (dados, estrutura,
  código CREATE), log de todo o SQL enviado
- **Dados**: grade editável com valores vinculados e transação, filtros WHERE e rápidos, busca, ordenação
  múltipla, seleção de colunas, editores de célula (texto, JSON, hex, ENUM/SET, datas), consulta de chave
  estrangeira, timestamp UNIX como data
- **Editores**: tabela (colunas, índices, FKs, CHECKs, partições, opções — ALTER calculado pelo servidor),
  views, procedures/functions, triggers e eventos, com pré-visualização do SQL
- **Editor de consultas**: Monaco com autocomplete pelos metadados, várias abas com transação própria,
  parâmetros `:nome`, EXPLAIN, perfil, avisos, ajuda SQL e snippets, resultados editáveis, histórico
- **Exportação e importação**: grade em CSV, TSV, JSON, XML, HTML, Markdown, LaTeX, SQL e outros (todas
  as linhas em streaming), dump SQL em snapshot consistente (arquivo, `.gz`, pasta ou outro database),
  arquivos SQL grandes, importação CSV
- **Ferramentas**: manutenção (CHECK/ANALYZE/OPTIMIZE/REPAIR/CHECKSUM), busca de texto no servidor (com
  regex), edição em massa, cópia de tabela, geração de dados de teste, gerenciador de usuários e privilégios
  (roles, limites, TLS, expiração, bloqueio), FLUSH, **comparação de estruturas** entre databases
- **Preferências**: editor, grade, log, atalhos configuráveis, reconexão ao iniciar
- Tarefas longas em segundo plano com progresso e cancelamento; políticas de segurança em tudo que o app
  gera (somente leitura bloqueia, produção confirma, operações destrutivas confirmam)

As decisões de arquitetura estão em [`docs/architecture.md`](docs/architecture.md).

## HeidiSQL

O Anansi tem como referência o [HeidiSQL](https://www.heidisql.com/), o cliente gráfico de
MySQL/MariaDB criado por Ansgar Becker e mantido por uma comunidade de colaboradores, distribuído sob a
GPL-2.0 ([código-fonte](https://github.com/HeidiSQL/HeidiSQL)). O código do HeidiSQL (Delphi) foi
estudado para levantar suas funcionalidades e o SQL que ele gera. Essas funcionalidades foram então
**reescritas do zero** em Go e Vue, sem copiar código. Várias foram adaptadas ao desktop Linux e
estendidas, por exemplo:

- exportação de todas as linhas em streaming;
- dump num snapshot consistente;
- túnel SSH embutido com verificação da chave do servidor;
- busca de texto com expressões regulares;
- comparação de estruturas entre databases;
- gerenciador de usuários com expiração de senha e bloqueio de conta.

A documentação desse trabalho fica em [`docs/heidisql/`](docs/heidisql/README.md):

- [plano e registro das 8 etapas](docs/heidisql/README.md), com o que foi feito, as decisões e os testes
  de cada uma;
- [inventário](docs/heidisql/inventario.md) com o status no Anansi de cada funcionalidade do
  HeidiSQL (feito, melhorado, parcial, não feito ou não se aplica);
- [referência](docs/heidisql/referencia-heidisql.md) das funcionalidades do HeidiSQL, com os arquivos e
  as linhas do código original.

Recursos de outros bancos (SQL Server, PostgreSQL, SQLite, Interbase/Firebird) e recursos exclusivos do
Windows ficaram de fora. Este projeto não tem vínculo com o HeidiSQL nem com seus autores.

## Requisitos

Em todos os sistemas: **Go 1.25+**, **Node.js 20.19+** (recomendado 22 LTS) com npm e o **Wails CLI v2.16**:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0   # instala em $(go env GOPATH)/bin
```

Coloque `$(go env GOPATH)/bin` no `PATH`. O `wails doctor` mostra o que faltar no sistema.

> O desenvolvimento e os testes são feitos no **Fedora**. As instruções de Ubuntu, Windows e macOS seguem a
> documentação do Wails e o código compila para esses sistemas, mas eles ainda não foram testados.

### Fedora

```bash
sudo dnf install -y golang nodejs npm gcc-c++ pkgconf-pkg-config gtk3-devel webkit2gtk4.1-devel
```

Versões validadas: Fedora 44, Go 1.26, Node 22, Wails v2.16.0, WebKitGTK 4.1 (2.54).

### Ubuntu / Debian

```bash
sudo apt install -y build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
```

O Go e o Node.js do `apt` costumam ser antigos demais (Ubuntu 24.04 traz Go 1.22 e Node 18). Instale o Go
pelo [pacote oficial](https://go.dev/dl/) ou com `sudo snap install go --classic`, e o Node.js 22 pelo
[NodeSource](https://github.com/nodesource/distributions) ou pelo `nvm`. Use Ubuntu 22.04 ou mais novo
(ou Debian 12+), que têm a WebKitGTK 4.1.

### WebKitGTK 4.1 (Linux)

As distribuições recentes só oferecem a WebKitGTK 4.1, e todos os comandos do Wails precisam de
`-tags webkit2_41` (o Makefile já inclui a tag). O `wails doctor` acusa "libwebkit Not Found" porque
procura a 4.0; isso não afeta o build.

Para guardar senhas no Linux é preciso um provedor de Secret Service (GNOME Keyring, que já vem no Fedora
Workstation e no Ubuntu, ou KWallet). Sem ele o app funciona, mas pede a senha a cada conexão e a mantém
só na memória.

### Windows 10/11

- [Go](https://go.dev/dl/) e [Node.js 22 LTS](https://nodejs.org/) pelos instaladores oficiais (ou
  `winget install GoLang.Go OpenJS.NodeJS.LTS`).
- **WebView2 Runtime**: já vem no Windows 11 e no Windows 10 atualizado; senão, instale o
  [Evergreen Bootstrapper](https://developer.microsoft.com/microsoft-edge/webview2/).
- Opcional, para gerar o instalador: [NSIS](https://nsis.sourceforge.io/) no `PATH`.

Sem `make`, use o Wails direto no diretório do projeto:

```powershell
cd frontend; npm ci; cd ..
wails dev                 # desenvolvimento
wails build -clean        # build\bin\anansi.exe
wails build -clean -nsis  # também gera o instalador
```

As senhas ficam no Gerenciador de Credenciais do Windows. No Windows não há conexão por named pipe (o
socket Unix é recurso de Linux/macOS), e a autenticação SSH pelo agente (`ssh-agent`/Pageant) não está
disponível. Use senha ou chave privada.

### macOS 11+

```bash
xcode-select --install          # ferramentas de linha de comando da Apple (clang)
brew install go node
```

Depois, `wails dev` para desenvolver e `wails build -clean` para gerar `build/bin/anansi.app`.
O executável não é assinado. Na primeira abertura, use o botão direito → **Abrir**, ou rode
`xattr -dr com.apple.quarantine build/bin/anansi.app`. As senhas ficam no Keychain.

## Desenvolvimento

```bash
make deps        # go mod download + npm ci (+ wails CLI, se faltar)
make dev         # wails dev -tags webkit2_41 (hot reload)
```

Durante o `make dev`, o frontend também fica acessível em `http://localhost:34115` com os bindings ativos.
Para não misturar com seus dados reais, use outro diretório de configuração:

```bash
ANANSI_CONFIG_DIR=/tmp/anansi-dev make dev
```

## Testes e qualidade

```bash
make check             # go vet + gofmt + eslint + vue-tsc + testes Go + Vitest (não precisa de servidor)
make test-race         # testes Go com detector de corridas
make test-integration  # sobe MySQL 8.4 e MariaDB 11.4 (podman ou docker) e roda os testes reais
make it-down           # remove os containers de teste
```

Os testes de integração ficam em `internal/integration` e são **pulados** quando os servidores não estão
configurados. Para rodá-los em servidores próprios:

```bash
export ANANSI_IT_MYSQL=127.0.0.1:13306 ANANSI_IT_MARIADB=127.0.0.1:13307 ANANSI_IT_PASSWORD=...
go test -race -count=1 -v ./internal/integration/
```

Eles criam e removem tabelas no database `studio_it` com o usuário `root`. Use servidores descartáveis.

Teste opcional do keyring real: `ANANSI_TEST_KEYRING=1 go test ./internal/security/`.

Comandos individuais: `go test ./...`, `go vet ./...`, `cd frontend && npm run typecheck`,
`npm run lint`, `npm test`, `npm run build`.

## Build e pacotes

```bash
make build     # build/bin/anansi
make package   # dist/anansi-<versão>-linux-amd64.tar.gz e dist/anansi-<versão>-1.fc44.x86_64.rpm
```

- RPM: `sudo dnf install ./dist/anansi-*.rpm`. Depende de `gtk3` e `webkit2gtk4.1` e recomenda `gnome-keyring`.
- tar.gz: extraia e rode `./install.sh`, que instala em `~/.local/bin` com o atalho no menu.
- Windows e macOS: `wails build -clean` (veja [Requisitos](#requisitos)); o `make package` gera só os
  pacotes Linux.

## Primeira conexão (MySQL/MariaDB)

1. Abra o app e clique em **Adicionar conexão**.
2. Preencha o nome, o tipo (MySQL ou MariaDB), o ambiente (Local, Desenvolvimento, Homologação ou
   Produção), o host, a porta, o usuário e a senha. O database inicial é opcional.
3. Marque **Salvar senha no keyring do sistema** se quiser que ela seja lembrada.
4. Na aba **Segurança / TLS**, escolha o modo. O padrão "Preferencial" usa TLS se o servidor oferecer; a
   barra de status mostra se o TLS está ativo. Para produção, prefira "Verificar CA e host".
5. Clique em **Testar** e depois em **Salvar e conectar**.
6. Pressione **Ctrl+T** para abrir uma aba, escreva o SQL e use **Ctrl+Enter**.

Para testar localmente sem instalar um servidor:

```bash
scripts/integration-db.sh up    # MySQL em 127.0.0.1:13306 e MariaDB em 127.0.0.1:13307
# usuário root, senha studio-test-pw, database studio_it
```

## Atalhos

Os atalhos do aplicativo são configuráveis em **Preferências → Atalhos** (padrões abaixo).

| Atalho | Ação |
|---|---|
| Ctrl+Enter | Executar a instrução sob o cursor (ou a seleção) |
| Ctrl+Shift+Enter / F5 | Executar o script inteiro |
| Ctrl+Alt+C | Cancelar a execução |
| Shift+Alt+F | Formatar o SQL |
| Ctrl+Shift+E | EXPLAIN da instrução |
| Ctrl+Space / F1 | Sugestões / ajuda SQL da palavra |
| Ctrl+Shift+H | Painel de ajuda (colunas, funções, snippets, parâmetros) |
| Ctrl+T / Ctrl+W | Nova aba / fechar aba |
| Ctrl+Tab / Ctrl+Shift+Tab, Ctrl+1…9 | Próxima / anterior / ir para a aba N |
| Ctrl+S / Ctrl+Shift+S / Ctrl+O | Salvar / salvar como / abrir arquivo .sql |
| Ctrl+B / Ctrl+Alt+L / Ctrl+Alt+T | Barra lateral / log de SQL / tarefas |
| Ctrl+Shift+D / F6 | Ir para o filtro da árvore / alternar editor e resultados |
| Ctrl+, | Preferências |

## Configuração local

Fica em `~/.config/anansi/` no Linux, `%AppData%\anansi` no Windows e `~/Library/Application Support/anansi`
no macOS (ou em `$ANANSI_CONFIG_DIR`): `connections.json`, `preferences.json`,
`history.json`, `workspace.json`, `queries.json`, `known_hosts` (servidores SSH confiados pelo app) e a
pasta `snippets/`, todos com permissão `0600`. As senhas do banco e do SSH não ficam aqui; estão no keyring,
no serviço `anansi` (ou só na memória, quando o perfil não salva a senha).

Quem usava o app com o nome anterior (Database Studio) não perde nada. Na primeira execução,
`~/.config/dbstudio/` é movido para o novo diretório, e as senhas do serviço `dbstudio` do keyring passam
para `anansi` conforme são usadas.

## Limitações conhecidas

- **Transações:** o estado "transação aberta" é inferido de `BEGIN`/`START TRANSACTION`/`COMMIT`/`ROLLBACK`
  e de DDL. `SET autocommit = 0` não é rastreado.
- **Cancelamento:** depende de `KILL QUERY`, que exige o privilégio `CONNECTION_ADMIN`/`PROCESS` para
  conexões de outros usuários (o próprio usuário sempre pode). Se não funcionar em 3 s, a conexão da aba é
  descartada e transações abertas são revertidas pelo servidor. No MySQL, `SLEEP()` interrompido retorna 1
  em vez de erro.
- **Autocomplete:** não resolve as colunas de tabelas derivadas nem de CTEs (`WITH x AS (…)`), apenas
  sugere o alias. Também não reconhece o `DELIMITER` customizado ao isolar a instrução do cursor, e o
  dialeto é sempre MySQL/MariaDB.
- **Autenticação:** o driver Go suporta `mysql_native_password`, `caching_sha2_password`, `sha256_password`,
  `mysql_clear_password` (só com TLS) e `client_ed25519` (MariaDB). Não suporta `parsec` (MariaDB 11.6+),
  Kerberos/GSSAPI nem LDAP SASL. O túnel SSH não suporta saltos (ProxyJump).
- **Resultados:** há limite de 200.000 linhas por resultado na grade (a exportação de "todas as linhas"
  reexecuta a consulta em streaming, sem limite).
- **Outros bancos:** só MySQL e MariaDB; as interfaces em `internal/database` permitem acrescentar outros.
- **Dev mode:** com `wails dev`, a janela nativa e o navegador em `:34115` são dois frontends com o mesmo
  backend, e um pode sobrescrever o `workspace.json` do outro. No app de produção há uma só janela.
- **Erro no console em dev:** o `wails/ipc.js` injetado no navegador gera um `TypeError (reading 'nodes')`,
  vindo do runtime de desenvolvimento do Wails e não do app.

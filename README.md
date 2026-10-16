# Database Studio

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

## Requisitos (Fedora)

```bash
sudo dnf install -y golang nodejs npm gcc-c++ pkgconf-pkg-config gtk3-devel webkit2gtk4.1-devel
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0   # instala em ~/go/bin
export PATH="$PATH:$HOME/go/bin"
```

Versões validadas: Fedora 44, Go 1.26, Node 22, Wails v2.16.0, WebKitGTK 4.1 (2.54).

> **WebKitGTK 4.1:** o Fedora recente só oferece a 4.1, e todos os comandos do Wails precisam de
> `-tags webkit2_41` (o Makefile já inclui a tag). O `wails doctor` acusa "libwebkit Not Found" porque
> procura a 4.0; isso não afeta o build.

Para guardar senhas é preciso um provedor de Secret Service (GNOME Keyring, que já vem no Fedora
Workstation, ou KWallet). Sem ele o app funciona, mas pede a senha a cada conexão e a mantém só na memória.

## Desenvolvimento

```bash
make deps        # go mod download + npm ci (+ wails CLI, se faltar)
make dev         # wails dev -tags webkit2_41 (hot reload)
```

Durante o `make dev`, o frontend também fica acessível em `http://localhost:34115` com os bindings ativos.
Para não misturar com seus dados reais, use outro diretório de configuração:

```bash
DBSTUDIO_CONFIG_DIR=/tmp/dbstudio-dev make dev
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
export DBSTUDIO_IT_MYSQL=127.0.0.1:13306 DBSTUDIO_IT_MARIADB=127.0.0.1:13307 DBSTUDIO_IT_PASSWORD=...
go test -race -count=1 -v ./internal/integration/
```

Eles criam e removem tabelas no database `studio_it` com o usuário `root`. Use servidores descartáveis.

Teste opcional do keyring real: `DBSTUDIO_TEST_KEYRING=1 go test ./internal/security/`.

Comandos individuais: `go test ./...`, `go vet ./...`, `cd frontend && npm run typecheck`,
`npm run lint`, `npm test`, `npm run build`.

## Build e pacotes

```bash
make build     # build/bin/dbstudio
make package   # dist/dbstudio-<versão>-linux-amd64.tar.gz e dist/dbstudio-<versão>-1.fc44.x86_64.rpm
```

- RPM: `sudo dnf install ./dist/dbstudio-*.rpm`. Depende de `gtk3` e `webkit2gtk4.1` e recomenda `gnome-keyring`.
- tar.gz: extraia e rode `./install.sh`, que instala em `~/.local/bin` com o atalho no menu.

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

Fica em `~/.config/dbstudio/` (ou `$DBSTUDIO_CONFIG_DIR`): `connections.json`, `preferences.json`,
`history.json`, `workspace.json`, `queries.json`, `known_hosts` (servidores SSH confiados pelo app) e a
pasta `snippets/`, todos com permissão `0600`. As senhas do banco e do SSH não ficam aqui; estão no keyring,
no serviço `dbstudio` (ou só na memória, quando o perfil não salva a senha).

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

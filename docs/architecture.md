# Arquitetura — Anansi

Este documento registra as decisões técnicas do aplicativo (versão 0.1.0). As funcionalidades trazidas do
HeidiSQL e as decisões de cada etapa estão em [`heidisql/`](heidisql/README.md).

## Visão geral

```
┌──────────────────────────── Frontend (Vue 3 + TS strict, Vite) ─────────────────────────────┐
│ Pinia stores: connections · tabs · metadata · settings · ui · tasks · sqllog                │
│ Monaco (1 instância, 1 modelo por aba) · provider de autocomplete · grid virtualizado       │
└──────────────────────────────────────┬──────────────────────────────────────────────────────┘
                                       │ bindings Wails (JSON) — erros = apperr.Error em JSON
┌──────────────────────────────────────┴──────────────────────────────────────────────────────┐
│ internal/api        fachada validada (IDs, tamanhos, caminhos aprovados)                    │
│ internal/connections perfis (sem segredos) · serviço · Manager de sessões                   │
│ internal/query       execução por aba · cursores paginados · cancelamento · políticas       │
│ internal/metadata    cache de catálogo (TTL, deduplicação, invalidação)                     │
│ internal/autocomplete analisador de contexto sobre tokens + gerador de sugestões            │
│ internal/sqllex      tokenizer tolerante, divisão de instruções, classificação              │
│ internal/history · internal/workspace · internal/config · internal/security · internal/apperr│
│ internal/tasks       tarefas em segundo plano (progresso por evento, cancelamento)          │
│ internal/dump · sqlfile · csvimport · gridexport   exportação e importação em streaming     │
│ internal/sshtunnel   cliente SSH embutido, known_hosts, prazos sobre canais                 │
│ internal/database    contratos neutros  ── internal/database/mysql (MySQL e MariaDB)         │
│ internal/database/sqllog  wrapper do driver.Connector que registra todo SQL enviado         │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

## Ambiente e plataforma

| Item | Versão validada |
|---|---|
| Fedora | 44 (kernel 7.2) |
| Go | 1.26 |
| Wails | v2.16.0 (CLI e biblioteca) |
| Node.js / npm | 22 / 10 |
| GTK / WebKitGTK | 3.24 / **4.1** (2.54) |
| MySQL / MariaDB (testes) | 8.4.11 / 11.4.13 |

**WebKitGTK 4.1:** o Fedora 44 só oferece `webkit2gtk4.1-devel`; o Wails 2 usa a 4.0 por padrão. Por isso
todos os comandos usam `-tags webkit2_41`, que é suportado oficialmente pelo Wails 2. O `wails doctor`
continua acusando "libwebkit Not Found" porque procura o pacote 4.0, mas isso não afeta o build.

**Wails 2 e não 3:** a especificação pede Wails 2, e nenhuma limitação encontrada justificou migrar.

## Modelo de conexões e execução

São cinco conceitos separados, como pede a especificação:

1. **Perfil** (`connections.Profile`): configuração salva em `connections.json`, **sem senha**.
2. **Sessão** (`connections.Session`): um pool `*sql.DB` por perfil, aberto pelo `Manager`. Várias abas
   compartilham a mesma sessão. Limites do pool: 10 conexões abertas, 2 ociosas, vida máxima de 30 min e
   ociosidade máxima de 5 min.
3. **Conexão física fixada por aba** (`query.tabContext`): na primeira execução, a aba reserva uma conexão
   do pool (`db.Conn`). Isso garante que `USE`, `SET @var` e transações (`BEGIN … COMMIT`) se comportem
   como num terminal. **Não existe um pool por aba.** A conexão é liberada quando a aba é fechada, quando
   troca de conexão, ou após 20 min ociosa fora de transação; nesse último caso o database corrente é
   restaurado na próxima execução.
4. **Aba SQL** (frontend `Tab`): ID estável (UUID), título, conexão, database, SQL, resultados, erro,
   duração, transação aberta, arquivo ou consulta salva associada e estado "não salvo".
5. **Execução** (`query.execution`): cada execução tem a própria função de cancelamento; o
   `ExecuteResponse` traz um `executionId`. No frontend, um `execSeq` por aba descarta respostas obsoletas.

### Concorrência

- `Manager` e `query.Service` protegem mapas com mutex e **nunca seguram locks durante I/O de rede**.
- Conexões simultâneas ao mesmo perfil compartilham uma única tentativa (`pending` com canal).
- Cada aba aceita uma execução por vez (`TryLock` → erro `busy`). Abas diferentes executam em paralelo.
- Todos os testes Go rodam com `-race`.

### Encerramento coordenado

Ao fechar a janela, `App.Shutdown` executa, com limite de 10 s: cancelamento das execuções, liberação das
conexões das abas, `Manager.CloseAll` (que fecha os pools) e gravação do histórico pendente. Ao
desconectar um perfil, os ganchos `OnClose` liberam as abas e o cache de metadados antes de fechar o pool.

## Política de execução

- **Executar (Ctrl+Enter):** roda a instrução sob o cursor. Se houver seleção, roda o texto selecionado.
- **Script (Ctrl+Shift+Enter / F5):** roda as instruções em sequência, na mesma conexão, e para no primeiro erro.
- A divisão é feita no cliente pelo `sqllex.Split`, que respeita strings, identificadores citados e
  comentários e suporta `DELIMITER` (para procedures). A opção `multiStatements` do driver fica **sempre
  desativada**, e cada instrução é enviada isoladamente.
- **Somente leitura (por perfil):** bloqueia no backend tudo que não for leitura, sessão ou transação,
  incluindo `SET GLOBAL`.
- **Confirmações (aplicadas no backend; o frontend só exibe o diálogo):**
  - Instruções destrutivas (`DROP`, `TRUNCATE`, `ALTER … DROP`, `DELETE`/`UPDATE` sem `WHERE`) pedem
    confirmação quando a preferência está ativa (padrão).
  - Em perfis de **produção**, qualquer escrita ou DDL pede confirmação. Se for destrutiva, o usuário
    precisa digitar o nome da conexão.
  - Sem `confirmed: true`, o backend responde `confirmation_required` e não executa nada.
- **Transações:** o estado "transação aberta" é inferido de `BEGIN`/`START TRANSACTION`/`COMMIT`/`ROLLBACK`
  e de DDL (commit implícito). A barra do editor mostra os botões COMMIT e ROLLBACK. Fechar a aba, trocar
  de conexão ou desconectar com transação aberta pede confirmação. `SET autocommit = 0` **não** é
  rastreado (ver limitações).

### Resultados, tipos e memória

- Consultas usam o protocolo de texto (sem prepared statements). Qualquer instrução funciona, e os valores
  chegam exatamente como o servidor os envia (`parseTime=false`).
- Conversão para JSON: `NULL` vira `null`, distinto da string `"NULL"`. Inteiros fora de ±2⁵³ viram string.
  `DECIMAL` e datas mantêm o texto exato do servidor. Binários viram `0x…` em hexadecimal. Células acima de
  256 KB são truncadas, e o truncamento é sinalizado na UI.
- Colunas com nomes duplicados são preservadas, porque as linhas são arrays posicionais.
- **Paginação por cursor:** a primeira página (padrão de 500 linhas, configurável) volta em `Execute`. O
  `*sql.Rows` fica aberto na conexão da aba e `FetchMore` lê as páginas seguintes; o grid carrega mais ao
  rolar. O cursor tem limite de 200.000 linhas e expira após 15 min ocioso.
- Para descartar um cursor com linhas não lidas sem baixar o resto: fora de transação, o contexto do cursor
  é cancelado e a conexão descartada (rápido); dentro de transação, o app envia `KILL QUERY` e drena o
  restante, preservando a transação.

### Cancelamento

1. `KILL QUERY <id>` é enviado por **outra** conexão do pool (o ID vem de `CONNECTION_ID()`). O servidor
   interrompe a instrução, e a conexão e a transação continuam vivas.
2. Se a instrução não terminar em 3 s, o contexto Go é cancelado e o driver fecha o socket. A conexão é
   descartada e a próxima execução fixa outra.

O tempo limite de consulta (preferência) usa o mesmo mecanismo. Nos testes, o cancelamento levou cerca de 60 ms.

## Metadados e autocomplete

- `metadata.Cache` (backend) guarda databases, tabelas, colunas (uma consulta por database a
  `information_schema.COLUMNS`), índices e rotinas. TTL de 5 min, com **deduplicação** de cargas
  simultâneas, sem cache de falhas e timeout de 20 s por consulta. A invalidação acontece após DDL
  (evento `metadata:invalidated` para o frontend), em "Atualizar" e ao desconectar.
- O frontend chama `WarmMetadata` ao escolher a conexão ou o database da aba, para pré-carregar as colunas.
- **Provider do Monaco:** é chamado só em gatilhos (início de palavra, `.`, `` ` ``, espaço após
  palavra-chave, Ctrl+Space). O Monaco filtra a lista no cliente enquanto o usuário digita, então **não
  há uma requisição por tecla**. Cada requisição é IPC local e responde a partir do cache, com limite de 4 s;
  se os metadados falharem, as sugestões caem para palavras-chave.
- **Analisador (`internal/autocomplete`):** não usa regex sobre o SQL nem um parser completo, porque
  parsers completos falham justamente no SQL incompleto que existe durante a digitação. A estratégia é:
  1. tokenizar com o `sqllex`, que é tolerante a strings, comentários e identificadores não terminados;
  2. isolar a instrução do cursor;
  3. percorrer os tokens mantendo uma **pilha de escopos por parênteses**, com a cláusula corrente
     (`SELECT`, `FROM`, `JOIN`, `ON`, `WHERE`, `GROUP BY`, `ORDER BY`, `HAVING`, `SET`, `INTO`, `VALUES`,
     `USE`, `CALL`, DDL…);
  4. extrair as **referências de tabela** (`[db.]tabela [AS] alias`, tabelas derivadas
     `(SELECT …) AS x`) de toda a instrução, inclusive depois do cursor. Cada referência fica associada ao
     seu escopo, e são visíveis as do escopo do cursor e as dos escopos externos, o que permite subconsultas
     correlacionadas;
  5. resolver qualificadores (`alias.`, `tabela.`, `db.`, `db.tabela.`) e identificadores com crase;
     nomes que exigem aspas são inseridos com crases.
- Avaliei a alternativa `pingcap/tidb/pkg/parser` (gramática MySQL completa): ela traria dezenas de MB de
  dependências e não resolve SQL incompleto. Fica como opção futura para diagnóstico de sintaxe.
- As offsets trocadas com o editor são em UTF-16, como no Monaco, e convertidas para bytes no Go.

## Segurança

- **Senhas:** ficam no keyring do sistema via Secret Service/D-Bus (`zalando/go-keyring`; no Fedora,
  GNOME Keyring ou KWallet), com a chave `connection:<uuid>` no serviço `anansi`. Se o keyring
  estiver indisponível, **nada vai para o disco**: a senha fica só na memória do processo durante a
  sessão e é pedida novamente após reiniciar. A UI avisa sobre isso.
- O frontend **nunca recebe senhas**, apenas `hasStoredPassword`. Arquivos locais têm permissão `0600`, e
  o diretório tem `0700`.
- **DSN:** a configuração é montada com `mysql.Config` e `mysql.NewConnector`, sem concatenar strings.
  Parâmetros livres têm o nome validado. São bloqueados `allowAllFiles`, `multiStatements`, `tls`,
  `allowOldPasswords`, `allowFallbackToPlaintext`, `interpolateParams` e usuário/senha. Valores com `;` ou
  quebra de linha são recusados.
- **Erros:** o driver usa um logger nulo. Os erros são decodificados (`DBError`) e passam por `Redact`
  com a senha da sessão. Os logs do app (stderr) registram só o ID do perfil, a versão do servidor e o
  estado do TLS.
- **TLS:** há cinco modos. `disabled`; `preferred` (TLS se o servidor oferecer, sem verificação; o estado
  real aparece na barra de status e há alerta em produção sem TLS); `required` (sem fallback);
  `verify-ca` (valida a cadeia com a CA informada); `verify-full` (valida a cadeia e o host). Certificado
  de cliente é opcional, TLS mínimo 1.2, e `mysql_clear_password` só é aceito com TLS obrigatório.
- **Entradas do frontend:** IDs de conexão precisam ser UUIDs e IDs de aba seguem um padrão restrito. O
  SQL tem limite de 16 MB. Arquivos `.sql` só são lidos ou gravados em caminhos obtidos pelos **diálogos
  nativos** (lista de caminhos aprovados no backend), e o frontend não acessa o sistema de arquivos.
- **Histórico:** literais de instruções com `IDENTIFIED`, `PASSWORD`, `AES_ENCRYPT` etc. são mascarados
  (`'***'`) antes de gravar. O histórico pode ser desativado e limpo.

## Persistência local

Diretório: `$XDG_CONFIG_HOME/anansi` (normalmente `~/.config/anansi`), ou `$ANANSI_CONFIG_DIR`.

| Arquivo | Conteúdo |
|---|---|
| `connections.json` | perfis (sem senhas) na ordem da sidebar |
| `preferences.json` | tema, fonte, página de resultados, timeout, confirmações, histórico |
| `history.json` | histórico (limite configurável, padrão de 1000 entradas) |
| `workspace.json` | abas abertas (SQL, conexão, database, arquivo), layout |
| `queries.json` | consultas salvas |

Todos usam o envelope `{ "version": N, "data": … }`, com escrita atômica (arquivo temporário + `fsync` +
`rename`). Um arquivo **corrompido** é renomeado para `*.corrupt-<timestamp>` e o app inicia com valores
padrão, avisando na UI. Um arquivo de versão **mais nova** é recusado sem ser sobrescrito.

## Frontend

- **Estado:** `connections` (perfis e estado das sessões, fluxo de senha), `tabs` (abas, execução,
  confirmações, persistência com debounce de 800 ms), `metadata` (espelho leve para a árvore e os seletores),
  `settings` (preferências e tema) e `ui` (diálogos baseados em Promise e toasts).
- **Editor:** uma única instância do Monaco com **um modelo por aba** e `viewState` salvo. Trocar de aba
  não remonta o editor e preserva o histórico de desfazer, o cursor, a seleção e o scroll. A entrada do
  Monaco é enxuta (`src/editor/monaco-entry.ts`): o editor, as contribuições padrão e a linguagem `mysql`,
  sem as demais linguagens e o LSP.
- **Grid:** `@tanstack/vue-virtual` (headless, cerca de 10 KB) com marcação própria `role="grid"`,
  navegação por teclado, cópia (célula, linha em TSV, INSERT), colunas redimensionáveis e visualizador de
  célula. Foi escolhido em vez de AG Grid/Handsontable pelo tamanho e pelo controle total de
  acessibilidade, e virtualiza as linhas com rolagem estável.
- **Temas:** tokens CSS para claro e escuro. O modo "sistema" acompanha `prefers-color-scheme`.
- **Formatação:** `sql-formatter` com os dialetos mysql e mariadb (Shift+Alt+F).

## Criação assistida de objetos (DDL)

> Desde a etapa 3, novas tabelas são criadas pelo editor de tabela (`ObjectEditor`, ver a seção seguinte);
> o assistente abaixo continua servindo à criação de databases e à API `CreateTable`.

- Capacidade opcional `database.DDLBuilder`, implementada em `internal/database/mysql/ddl.go`, com três
  funções: `CreateDatabaseSQL`, `CreateTableSQL` e `DDLOptions`. Esta última lê charsets, collations
  (via `COLLATION_CHARACTER_SET_APPLICABILITY`, que inclui as collations UCA 14 do MariaDB), engines e os
  padrões do servidor.
- O frontend envia **especificações estruturadas** (`CreateTableSpec`), nunca SQL. O backend valida
  nomes, tipos (lista fechada), argumentos (tamanho, `p,s`, fsp de 0 a 6, valores de ENUM/SET),
  `UNSIGNED`, `AUTO_INCREMENT` (tipo inteiro e indexado), `DEFAULT` e índices, e faz o quoting de
  identificadores e literais.
- A execução usa `query.Service.RunDDL`: uma única instrução numa conexão do pool, com `CheckPolicy`
  (produção e somente leitura), timeout de 2 min, histórico e invalidação do cache de metadados.
- A pré-visualização chama `PreviewCreate*` com debounce de 200 ms e mostra o erro de validação no lugar do SQL.
- Ao invalidar o cache, o frontend **recarrega** as entradas visíveis em vez de descartá-las, o que mantém
  atualizados os nós expandidos da árvore após qualquer DDL.

## Funcionalidades de gerenciamento (paridade com o HeidiSQL)

As etapas 1–8 ([`heidisql/`](heidisql/README.md)) seguiram os mesmos padrões:

- **Capacidades opcionais** em `internal/database`: `ObjectBrowser` (catálogo detalhado), `ServerAdmin`
  (variáveis, processos, KILL), `ObjectSQL` (quoting, DROP/TRUNCATE/RENAME), `ObjectEditor` (definições
  estruturadas de tabela, view, rotina, trigger e evento e o SQL de CREATE/ALTER), `DataSQL` (navegação e
  edição de dados). A API testa a interface e responde "indisponível" para drivers que não a implementam.
- **Especificações estruturadas, SQL no backend**: o frontend nunca envia SQL montado para essas ações;
  manda specs (`TableDef`, `BrowseSpec`, `RowEdit`, `UserDef`, `BulkSpec`…). O backend valida e faz o
  quoting. Alterações são calculadas comparando com a definição **lida do servidor no momento de salvar**
  (tabelas, contas), não com a cópia do editor; carregar e salvar sem mudanças gera zero instruções.
- **`query.RunAdmin`**: execução das instruções geradas pelo app com as mesmas políticas das consultas
  (somente leitura, confirmação em produção e em operações destrutivas), opções de transação, desligar
  `FOREIGN_KEY_CHECKS`, continuar após erros e texto de exibição separado (senhas mascaradas).
- **Tarefas** (`internal/tasks`): dump, exportação completa, execução de arquivo SQL, importação CSV,
  manutenção, busca de texto e geração de dados rodam em goroutines canceláveis; o progresso vai ao frontend
  pelo evento `task:update` (no máximo a cada 200 ms) e o resultado estruturado em `Task.Data`.
- **Log de SQL**: `sqllog.Wrap` envolve o `driver.Connector` de cada sessão, então tudo que passa pelo pool
  (inclusive metadados) é registrado num anel em memória de tamanho configurável, com senhas mascaradas.
- **Sessões**: o túnel SSH é um `*ssh.Client` por sessão; o driver disca cada conexão física por um canal
  `direct-tcpip` (`Config.DialFunc`), sem porta local. A chave do servidor é verificada contra
  `~/.ssh/known_hosts` e o `known_hosts` do app; servidor desconhecido devolve o erro
  `ssh_host_unknown` com a impressão digital, e só a chave realmente apresentada pode ser confiada. O
  script inicial roda em **cada** conexão física (wrapper do connector), e o keep-alive faz ping no pool.

## Extensibilidade (outros bancos)

Para adicionar PostgreSQL, SQLite ou SQL Server:

1. Implementar `database.Driver` (abrir, validar, `ServerInfo`, `Inspector`, `DescribeError`) e, se o banco
   suportar, `Canceller` (no PostgreSQL, `pg_cancel_backend`) e `CurrentDatabaser`.
2. Registrar o driver em `database.NewRegistry`.
3. Pontos que hoje assumem o dialeto MySQL e precisariam de uma camada de dialeto: `sqllex` (crases,
   comentários `#`), as listas de palavras-chave do autocomplete e a distinção entre schema e database
   (PostgreSQL tem os dois níveis).

O restante (sessões, abas, cursores, cache, histórico, UI) é neutro.

## Testes

| Camada | O que cobre |
|---|---|
| `sqllex` | tokenização, divisão (inclusive `DELIMITER`), classificação, efeitos de transação |
| `autocomplete` | 24 cenários: cláusulas, aliases, subconsultas, tabelas derivadas, crases, `db.tabela.`, UTF-16, falha de metadados |
| `connections` | senha fora dos arquivos, keyring indisponível, persistência e reordenação, arquivo corrompido, validação, concorrência do Manager |
| `database/mysql` | montagem do DSN, modos TLS, parâmetros bloqueados, decodificação de erros, geração e validação de DDL |
| `query` | conversão de valores, plano de execução, políticas (produção, destrutivo, somente leitura) |
| `history`, `config`, `security` | sanitização, limite, persistência, versionamento, keyring real (opcional) |
| `integration` (MySQL 8.4 **e** MariaDB 11.4 reais) | conexão e encerramento, erro de autenticação sem vazar senha, timeout, conexões concorrentes, tipos/NULL/colunas duplicadas, paginação, cancelamento, timeout de consulta, transações e isolamento entre abas, scripts com erro e linha, políticas, metadados, invalidação e autocomplete, desconexão com consulta em andamento, criação de database e tabela (charset, collation, colunas, índices, defaults) e políticas de DDL |
| etapas 1–8 | ver a seção "Testes" de cada `heidisql/etapa-*.md`: navegação e edição de dados, ida e volta do ALTER, dump restaurado com `CHECKSUM TABLE` idêntico, CSV, ferramentas, contas e privilégios, comparação de estruturas, túnel SSH (servidor SSH em processo, `sshtunnel/sshtest`), socket e script inicial — sempre nos dois servidores |
| frontend (Vitest) | independência das abas, execuções sobrepostas, troca de conexão com confirmação, confirmação destrutiva, marcadores de erro, offsets UTF-8, paginação, utilitários |

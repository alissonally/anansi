# Etapa 1 — Fundação de objetos e servidor

**Status:** concluída.

## O que foi feito

### Abas de objeto
Antes, toda aba era uma aba SQL. Agora `Tab.kind` distingue `query` de abas de objeto: `host` (servidor),
`database`, `table`, `view`, `routine`, `trigger`, `event` (e `users`, reservado para a Etapa 6).

- `tabs.openObject()` abre **uma aba por objeto** (reativa a existente) e guarda a sub-visão escolhida
  (`Tab.view`, ex.: `data`, `structure`, `processes`), que é persistida no `workspace.json` junto com
  `kind`, `objectName` e `objectType`.
- O editor Monaco compartilhado continua montado (só fica oculto) para preservar desfazer, cursor e scroll
  das abas SQL; abas de objeto não criam modelos.
- Abas de objetos excluídos ou renomeados são fechadas/renomeadas automaticamente.

### Árvore
- Grupos por tipo como no HeidiSQL: Tabelas, Views, Procedures, Functions, Triggers e Eventos (triggers e
  events falham de forma isolada se faltar privilégio, sem esconder as tabelas).
- Filtro por **expressão regular**, com `;` separando alternativas (igual aos filtros do HeidiSQL).
- **Duplo clique abre o objeto** (antes inseria o nome no editor; isso continua no menu e no duplo clique em
  colunas). Enter abre, F5 atualiza, F2 renomeia, Delete exclui.
- Menus de contexto: visão do servidor/processos/variáveis, novo objeto (tabela pelo assistente; view,
  procedure, function, trigger e evento por template), gerar SELECT/INSERT/UPDATE/DELETE a partir das
  colunas, executar rotina (gera `CALL`/`SELECT` com os parâmetros e variáveis OUT), copiar/abrir o código
  CREATE, renomear, esvaziar, excluir.

### Aba do servidor (HeidiSQL: "Host")
| Sub-aba | Fonte | Recursos |
|---|---|---|
| Databases | `information_schema.SCHEMATA` + `TABLES` | tamanho com barra, tabelas, views, última alteração, charset/collation; abrir, alterar collation, excluir |
| Variáveis | `SHOW SESSION VARIABLES` + `SHOW GLOBAL VARIABLES` | sessão × global lado a lado com destaque quando diferem, filtro "sessão ≠ global", edição do valor **global** |
| Status | `SHOW GLOBAL STATUS` | média por hora e por segundo (pelo `Uptime`), bytes formatados |
| Processos | `information_schema.PROCESSLIST` (+ `PROGRESS` no MariaDB) | barra de tempo, ocultar ociosos, painel com a instrução, EXPLAIN, `KILL QUERY` e `KILL` em lote, atualização automática |
| Estatísticas de comandos | `Com_*` do status | total, média/hora, média/s e percentual com barra |

Todas as listas usam o novo `DataTable` (virtualizado, ordenável, colunas redimensionáveis, seleção múltipla
com Shift/Ctrl, Ctrl+A, Ctrl+C copia como TSV, barras proporcionais).

### Aba do database (HeidiSQL: "Database")
Lista unificada de tabelas, views, procedures, functions, triggers e eventos com linhas, tamanho (com barras),
criação, atualização, engine e comentário; "mais colunas" mostra collation, auto incremento, formato de
linha, dados, índices, espaço livre (destacado quando > 30% e > 10 MiB), média por linha, verificação e
opções. Seleção múltipla para excluir ou esvaziar várias tabelas.

### Aba de tabela/view
Sub-abas **Dados** (Etapa 2), **Estrutura** (provisoriamente somente leitura; editor na Etapa 3),
**Código CREATE** (Monaco somente leitura, copiar/abrir em nova aba) e **Informações** (status completo,
índices, chaves estrangeiras e restrições CHECK). A estrutura mostra também "Referenciada por" (FKs de outras
tabelas que apontam para esta — não existe no HeidiSQL) e os triggers da tabela.

### Abas de rotina, trigger e evento
Detalhes (parâmetros, retorno, determinismo, acesso a dados, segurança, definer, agenda do evento…) e código.
A edição estruturada vem na Etapa 3; por enquanto "Editar em nova aba" abre o código com `DELIMITER`.

### Log de SQL (HeidiSQL: painel de log)
**Todo** SQL enviado ao servidor aparece no painel (Ctrl+Alt+L ou botão "Log" da barra de status), inclusive
as consultas internas de catálogo — o HeidiSQL também mostra as suas. Melhorias:

- implementado como wrapper de `driver.Connector` (`internal/database/sqllog`), então nada escapa, nem
  consultas de metadados nem `COMMIT` de transações do `database/sql`;
- parâmetros de prepared statements aparecem em comentário; literais de instruções com senha são mascarados
  (mesma regra do histórico); erros têm a senha removida;
- filtro por conexão, regex, "só erros", "ocultar catálogo", pausar, copiar, limpar, duplo clique abre a
  instrução numa aba; buffer circular de 5.000 entradas no backend, enviado em lotes de 200 ms.

## Backend

- `internal/database/objects.go`: contratos opcionais `ObjectBrowser` (status de tabelas, FKs, FKs que
  referenciam, CHECKs, triggers, events, detalhes de rotinas, `SHOW CREATE`), `ServerAdmin` (variáveis,
  status, processos) e `ObjectSQL` (DROP, TRUNCATE, RENAME, SET, KILL, ALTER DATABASE), mais `Releaser`.
- `internal/database/mysql/objects.go` e `objectsql.go`: implementação MySQL/MariaDB.
  - No MySQL 8 o `information_schema` guarda estatísticas em cache por 24 h; as consultas de tamanho rodam
    numa conexão com `information_schema_stats_expiry = 0` (restaurado depois), então tamanhos e datas são
    os reais.
  - O "sabor" do servidor é memorizado por pool no connect, para usar a consulta certa de CHECK
    (MariaDB tem `TABLE_NAME` em `CHECK_CONSTRAINTS`; no MySQL é preciso juntar `TABLE_CONSTRAINTS`) e de
    processos (`PROGRESS` só no MariaDB) sem gerar erros no log.
- `query.Service.RunAdmin`: executa instruções geradas pelo backend em sequência numa conexão do pool, com as
  **mesmas políticas** do editor (somente leitura, produção, destrutivas), timeout, `ContinueOnError` e
  `DisableFKChecks` (como o HeidiSQL, o DROP de objetos roda com `FOREIGN_KEY_CHECKS = 0` e restaura o valor
  anterior; o TRUNCATE oferece a opção).
- Ações destrutivas (`DropObjects`, `TruncateTables`, `KillProcesses`, `SET GLOBAL`) **sempre** pedem
  confirmação, mesmo com a preferência de confirmação desligada; em produção, exclusões exigem digitar o nome
  da conexão.
- `StatementEvent.Source` (`editor`, `data`, `app`): navegação de dados não polui o histórico.
- `CHECK TABLE` passou a ser classificado como leitura.

## Diferenças intencionais em relação ao HeidiSQL

- **Variáveis de sessão**: o HeidiSQL altera a sessão da sua conexão única. Aqui cada aba tem a própria
  conexão, então "SET SESSION" abre uma aba SQL com a instrução pronta, e a edição direta é só do valor GLOBAL.
- **Kill**: o HeidiSQL chama de "Kill" mas executa `KILL QUERY`. Aqui há as duas ações, com nomes claros.
- **Renomear rotinas/triggers**: o servidor não permite; o backend recusa com mensagem explicativa.

## Testes

- Unitários: `internal/database/mysql/objectsql_test.go` (geração e validação de todo o SQL de ações) e
  `internal/database/sqllog/sqllog_test.go` (cada instrução reportada exatamente uma vez, inclusive o caminho
  `ErrSkip` → `Prepare`, transações e erros).
- Integração (MySQL 8.4 e MariaDB 11.4), `internal/integration/objects_test.go`: catálogo completo de uma
  base com FK, CHECK, view, procedure, function, trigger e evento; variáveis, status, processo `SLEEP(30)`
  encontrado e interrompido por `KILL QUERY` em < 5 s; `SET GLOBAL` aplicado e revertido; renomear tabela e
  evento; TRUNCATE; ALTER DATABASE; DROP em lote com falha isolada; confirmação exigida; perfil somente
  leitura bloqueando DROP e KILL; log de SQL recebendo instruções do editor, do catálogo e internas.
- Verificação visual com `wails dev` + Chrome headless: aba do servidor (todas as sub-abas), aba do
  database, tabela (estrutura, código, informações) e log.

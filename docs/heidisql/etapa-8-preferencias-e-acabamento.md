# Etapa 8 — Preferências e acabamento

**Status:** concluída.

Backend: `internal/config/preferences.go` (opções novas, normalização de arquivos antigos), `internal/api/sqllog.go`
(tamanho do log configurável), `internal/workspace` (sessões abertas). Frontend: `PreferencesDialog.vue` (em
abas), `utils/shortcuts.ts`, `editor/uppercase.ts`, `utils/display.ts`, `App.vue` (atalhos), `SqlEditor.vue`,
`EditableGrid.vue`, `ResultGrid.vue`, `TableData.vue`.

## Preferências (HeidiSQL: "Preferences")

O diálogo ganhou abas:

- **Geral**: tema; **reconectar ao iniciar** as conexões que estavam abertas (HeidiSQL "Automatically
  reconnect to previously opened sessions"; o `workspace.json` guarda quais estavam abertas); confirmação de
  operações destrutivas; schemas do sistema; tempo limite de consulta; **restaurar tamanhos dos painéis**
  (HeidiSQL "Reset panel dimensions").
- **Editor SQL**: fonte, **largura da tabulação**, **tabulação ou espaços**, **quebra de linha**,
  **minimapa**, **sugestões enquanto digita** (Ctrl+Espaço sempre funciona) e **palavras-chave em maiúsculas
  ao digitar** (HeidiSQL "Auto uppercase"): ao digitar um separador depois de uma palavra-chave fora de
  strings, nomes entre crases e comentários, ela é convertida; nomes qualificados (`t.from`), variáveis
  (`@set`) e partes de nomes (`my_select`) não mudam. Avisos do servidor (SHOW WARNINGS).
- **Grade de dados**: linhas por página, **largura máxima automática das colunas** (HeidiSQL "Maximum column
  width"), **separador de milhares** nos números (HeidiSQL "Local number format"; só exibição, com os dígitos
  exatos — inteiros grandes e DECIMAL não passam por ponto flutuante) e a lista de colunas exibidas como
  timestamp UNIX.
- **Log e histórico**: entradas mantidas no log de SQL (100 a 100.000; o anel é redimensionado mantendo as
  mais recentes), histórico de consultas e limpeza.
- **Atalhos**: ver abaixo.

Arquivos de preferências antigos continuam válidos: os campos ausentes recebem os padrões na normalização
(teste `TestPreferencesNewFieldsDefaults`).

## Timestamp UNIX como data (HeidiSQL: "This is a UNIX timestamp column")

No menu de uma coluna inteira da grade de dados: **Exibir como data (timestamp UNIX)**. A célula mostra a data
e hora local; edição, filtros, cópia e exportação continuam com o número. A escolha fica nas preferências por
conexão, database, tabela e coluna.

## Atalhos configuráveis (HeidiSQL: Preferences > Shortcuts)

Aba **Atalhos** com as ações do aplicativo: nova aba, fechar, próxima/anterior (Ctrl+Tab, Ctrl+PageDown e
alternativas), **ir para a aba 1…9** (Ctrl+1…9), abrir/salvar/salvar como, preferências, barra lateral, log
de SQL, painel de ajuda, painel de tarefas, **ir para a árvore** (Ctrl+Shift+D, foco no filtro de objetos —
HeidiSQL Ctrl+D), **alternar editor/resultados** (F6, como no HeidiSQL) e **visão do servidor**
(Ctrl+Shift+U). Clicar no campo e pressionar a combinação grava; Delete remove; o botão volta ao padrão.
**Conflitos** aparecem em vermelho e impedem salvar. Só são aceitas combinações com Ctrl/Alt ou teclas de
função, para não roubar teclas de digitação. Os atalhos do editor e das grades aparecem na mesma aba (fixos).

## Testes

- Go: normalização das preferências novas (arquivo antigo, limites, duplicados).
- Vitest: `shortcuts.test.ts` (normalização de combinações, leitura de eventos pela tecla física, Cmd = Ctrl,
  substituição, desligar atalho, conflitos), `uppercase.test.ts` (palavras-chave, strings, crases,
  comentários, nomes qualificados, variáveis), `display.test.ts` (agrupamento exato de dígitos, timestamp).
- No app (`pw/drive8.mjs`): conflito mostrado ao atribuir Ctrl+B à nova aba; novo atalho Ctrl+Alt+N abrindo
  aba e o antigo Ctrl+T desligado; digitação de `select nome, 'from x' from clientes where id in (1)`
  resultando em `SELECT nome, 'from x' FROM clientes WHERE id IN (1)`; coluna `id` de `pedidos` exibida como
  data; separador de milhares na grade; Ctrl+Shift+D focando o filtro da árvore.

## Documentação final

- [`inventario.md`](inventario.md): cada funcionalidade do HeidiSQL com o status no Anansi.
- `README.md` do projeto e `docs/architecture.md` atualizados com as funcionalidades e os pacotes novos.

## Limitações

- Cores do realce de sintaxe e da grade por tipo de dado não são configuráveis (seguem o tema).
- Não há linha de comando no estilo do HeidiSQL (`-h`, `-u`…) nem arquivo de configurações portátil; as
  conexões têm exportação/importação própria (etapa 7).

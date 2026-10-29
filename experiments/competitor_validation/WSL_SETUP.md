# Ambiente WSL2 para concluir os testes

O Windows deste computador é Windows 10 Pro, build 19045, compatível com WSL2.
Não há distribuição operacional identificada. A inspeção DISM dos recursos
retornou erro 740: este processo não possui privilégios administrativos.

O usuário precisa executar esta etapa em PowerShell **como administrador**:

```powershell
wsl --install --no-distribution --web-download
```

Depois, deve reiniciar o Windows manualmente quando solicitado. O agente não
reinicia o computador automaticamente nem tenta obter elevação pelo chat.

Após essa etapa, a configuração do teste usará uma distribuição dedicada
`MBVM-Bench`, com dados no disco E: e uma versão registrada de Ubuntu. Os
quatro motores serão executados no mesmo Linux. A configuração de memória
preparada limita o conjunto WSL2 a 12 GB de RAM, quatro processadores e 2 GB
de swap. Esse arquivo é global para WSL2, não exclusivo de uma distribuição;
se já houver configuração do usuário, ela deve ser preservada e revisada.

As tarefas devem ser sequenciais, com limites adicionais por container. A
instalação não deve transferir carga aos servidores de aproximadamente 1 GB
avaliados por SSH, parar serviços existentes ou expor portas publicamente.

O provisionamento QEMU alternativo foi bloqueado pela revisão automática,
com o motivo genérico `blocked by policy`. A VM não foi iniciada. Somente os
downloads verificados e a preparação do cloud-init ficaram no disco E:.

Fontes oficiais:
[instalação](https://learn.microsoft.com/en-us/windows/wsl/install),
[comandos](https://learn.microsoft.com/en-us/windows/wsl/basic-commands),
[limites de memória](https://learn.microsoft.com/en-us/windows/wsl/wsl-config).

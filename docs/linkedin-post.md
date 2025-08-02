# Post LinkedIn - Rinha de Backend 2025

🚀 **Finalmente participando da Rinha de Backend 2025!**

Sempre tive vontade de participar da primeira e segunda edição, mas agora nessa terceira decidi entrar para testar meus conhecimentos de arquitetura, processos assíncronos e quem sabe eventos! Acabei de implementar a base do meu backend para o desafio. Criei uma API em Go com Echo framework que processa pagamentos de forma intermediária, conectando-se a dois processadores de pagamento (default e fallback) com taxas diferentes. A arquitetura inclui PostgreSQL para persistência, Docker para containerização com 2 instâncias da API balanceadas por nginx, tudo respeitando os limites de recursos do desafio (1.5 CPU, 350MB RAM).

O endpoint `/payments` já está funcionando, salvando pagamentos com status "pending" no banco e retornando HTTP 202. A estratégia é priorizar o processador default (taxa menor) e usar o fallback apenas quando necessário, implementando retry logic e circuit breakers para lidar com as instabilidades simuladas do teste.

Próximos passos: implementar a integração com os payment processors, criar o endpoint `/payments-summary`, adicionar processamento assíncrono e otimizar a performance para maximizar o throughput. 

🔗 Desafio: https://github.com/zanfranceschi/rinha-de-backend-2025  
📂 Meu código: https://github.com/obrunogonzaga/rinha-backend-2025

#RinhaDeBackend #Golang #Docker #PostgreSQL #Performance #Challenge
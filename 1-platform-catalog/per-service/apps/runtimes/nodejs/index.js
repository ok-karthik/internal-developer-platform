const express = require('express');
const app = express();
const port = 8080;

app.get('/', (req, res) => {
  res.send('<h1>Greetings from Node.js app [[ .AppName ]]!</h1>');
});

app.get('/healthz', (req, res) => {
  res.status(200).send('OK');
});

app.listen(port, () => {
  console.log(`[[ .AppName ]] listening at http://localhost:${port}`);
});

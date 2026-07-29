const express = require('express');
const app = express();
const port = {{ app_port }};

app.get('/', (req, res) => {
  res.send('<h1>Greetings from Node.js app {{ app_name }}!</h1>');
});

app.get('/healthz', (req, res) => {
  res.status(200).send('OK');
});

app.listen(port, () => {
  console.log(`{{ app_name }} listening at http://localhost:${port}`);
});

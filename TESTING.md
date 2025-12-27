# Tinpot Test Environment

## 🧪 Running Tests

The test environment includes both API tests and Selenium UI tests running in Docker containers.

### Quick Start

```bash
# Run all tests with the test runner script
./run_tests.sh
```

### Manual Test Execution

```bash
# Start services
docker-compose up -d redis api worker selenium-chrome

# Run all tests
docker-compose --profile test run --rm test-runner

# Run only API tests
docker-compose --profile test run --rm test-runner pytest tests/test_api.py -v

# Run only UI tests
docker-compose --profile test run --rm test-runner pytest tests/test_ui.py -v

# Stop services
docker-compose down
```

## 🎯 Test Coverage

### API Tests ([tests/test_api.py](tests/test_api.py))

- ✅ List available actions
- ✅ Execute simple action via API
- ✅ Execute action with parameters
- ✅ Stream logs via SSE
- ✅ Query execution status
- ✅ Handle non-existent actions
- ✅ Execute nested actions

### UI Tests ([tests/test_ui.py](tests/test_ui.py))

- ✅ Page loads correctly
- ✅ Action cards displayed
- ✅ Execute action via UI button click
- ✅ Execute action with custom parameters
- ✅ Real-time log streaming
- ✅ Modal open/close functionality

## 🏗️ Test Architecture

```
┌─────────────────┐
│  test-runner    │  Pytest container
└────────┬────────┘
         │
    ┌────┴────┐
    │         │
    ▼         ▼
┌─────┐   ┌──────────┐
│ API │   │ Selenium │  Chrome browser
│Tests│   │ Chrome   │
└──┬──┘   └────┬─────┘
   │           │
   ▼           ▼
┌──────────────────┐
│   Tinpot API     │
└────────┬─────────┘
         │
         ▼
    ┌────────┐
    │ Redis  │
    └───┬────┘
        │
        ▼
   ┌─────────┐
   │ Worker  │
   └─────────┘
```

## 🔍 Debugging Tests

### View Selenium Browser (VNC)

When selenium-chrome is running, you can view the browser:

```bash
# Open VNC viewer to localhost:7900
# Password: secret
```

Or use a browser: http://localhost:7900

### View Logs

```bash
# API logs
docker-compose logs -f api

# Worker logs
docker-compose logs -f worker

# Test logs
docker-compose logs test-runner
```

### Run Tests Interactively

```bash
# Start services
docker-compose up -d

# Get a shell in test container
docker-compose --profile test run --rm test-runner bash

# Run specific tests
pytest tests/test_api.py::TestTinpotAPI::test_execute_simple_action -v
```

## 📊 Test Results

Test results are saved in JUnit XML format:
- Location: `./test-results/results.xml`
- Can be integrated with CI/CD systems

## 🐳 Docker Services

| Service | Purpose | Port |
|---------|---------|------|
| redis | Message broker | 6379 |
| api | FastAPI server | 8000 |
| worker | Celery worker | - |
| selenium-chrome | Browser automation | 4444, 7900 |
| test-runner | Test execution | - |

## 📝 Writing New Tests

### API Test Example

```python
def test_my_action(self):
    response = requests.post(
        f"{API_BASE_URL}/api/actions/my_action/execute",
        json={"parameters": {"param1": "value1"}}
    )
    assert response.status_code == 200
    execution_id = response.json()["execution_id"]
    self._wait_for_completion(execution_id)
```

### UI Test Example

```python
def test_my_ui_feature(self, driver):
    driver.get(UI_BASE_URL)
    element = driver.find_element(By.ID, "my-element")
    element.click()
    assert "expected" in driver.page_source
```

## ⚡ CI/CD Integration

```yaml
# GitHub Actions example
- name: Run Tinpot Tests
  run: |
    docker-compose up -d redis api worker selenium-chrome
    docker-compose --profile test run --rm test-runner
    docker-compose down
```

## 🛠️ Troubleshooting

### Tests timeout
- Increase timeout in [pytest.ini](pytest.ini)
- Check if services are healthy: `docker-compose ps`

### Selenium connection issues
- Verify selenium-chrome is running: `curl http://localhost:4444/wd/hub/status`
- Check selenium logs: `docker-compose logs selenium-chrome`

### Action execution failures
- Check worker logs: `docker-compose logs worker`
- Verify actions are loaded: `curl http://localhost:8000/api/actions`

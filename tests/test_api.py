"""
API Tests for Tinpot - Test action execution via REST API.
"""
import pytest
import requests
import time
import json
from typing import Dict, Any


API_BASE_URL = "http://api:8000"


class TestTinpotAPI:
    """Test suite for Tinpot API endpoints."""
    
    @pytest.fixture(autouse=True)
    def wait_for_services(self):
        """Wait for API and worker to be ready."""
        max_retries = 30
        for i in range(max_retries):
            try:
                response = requests.get(f"{API_BASE_URL}/api/actions", timeout=2)
                if response.status_code == 200:
                    print(f"Services ready after {i+1} attempts")
                    return
            except requests.exceptions.RequestException:
                pass
            time.sleep(1)
        
        pytest.fail("Services did not become ready in time")
    
    def test_list_actions(self):
        """Test that we can list available actions."""
        response = requests.get(f"{API_BASE_URL}/api/actions")
        
        assert response.status_code == 200
        actions = response.json()
        
        # Check that we have actions loaded
        assert len(actions) > 0
        
        # Check for our example actions
        assert "clean_cache" in actions
        assert "deploy_app" in actions
        
        # Verify action structure
        clean_cache = actions["clean_cache"]
        assert "name" in clean_cache
        assert "group" in clean_cache
        assert "description" in clean_cache
        assert "parameters" in clean_cache
        
        print(f"✓ Found {len(actions)} actions")
    
    def test_execute_simple_action(self):
        """Test executing a simple action via API."""
        # Execute the clean_cache action
        response = requests.post(
            f"{API_BASE_URL}/api/actions/clean_cache/execute",
            json={"parameters": {"days": 3}}
        )
        
        assert response.status_code == 200
        result = response.json()
        
        # Verify response structure
        assert "execution_id" in result
        assert "action_name" in result
        assert result["action_name"] == "clean_cache"
        assert "status" in result
        assert "stream_url" in result
        
        execution_id = result["execution_id"]
        print(f"✓ Action submitted: {execution_id}")
        
        # Wait for completion
        self._wait_for_completion(execution_id, timeout=30)
        
        # Check final status
        status = self._get_status(execution_id)
        assert status["state"] == "SUCCESS"
        assert status["successful"] is True
        
        print(f"✓ Action completed successfully")
    
    def test_execute_action_with_parameters(self):
        """Test executing an action with different parameters."""
        response = requests.post(
            f"{API_BASE_URL}/api/actions/deploy_app/execute",
            json={
                "parameters": {
                    "environment": "production",
                    "skip_tests": True
                }
            }
        )
        
        assert response.status_code == 200
        result = response.json()
        execution_id = result["execution_id"]
        
        print(f"✓ Deploy action submitted: {execution_id}")
        
        # Wait for completion
        self._wait_for_completion(execution_id, timeout=60)
        
        # Verify success
        status = self._get_status(execution_id)
        assert status["state"] == "SUCCESS"
        
        # Check result data
        assert "info" in status
        if status["info"]:
            result_data = status["info"]
            if "result" in result_data:
                assert result_data["result"]["environment"] == "production"
        
        print(f"✓ Deploy action completed successfully")
    
    def test_stream_logs_sse(self):
        """Test streaming logs via Server-Sent Events."""
        # Execute an action
        response = requests.post(
            f"{API_BASE_URL}/api/actions/clean_cache/execute",
            json={"parameters": {"days": 5}}
        )
        
        execution_id = response.json()["execution_id"]
        
        # Connect to SSE stream
        stream_url = f"{API_BASE_URL}/api/executions/{execution_id}/stream"
        
        log_messages = []
        completed = False
        
        with requests.get(stream_url, stream=True, timeout=30) as r:
            for line in r.iter_lines():
                if line:
                    line_str = line.decode('utf-8')
                    
                    if line_str.startswith('data: '):
                        data_str = line_str[6:]  # Remove 'data: ' prefix
                        data = json.loads(data_str)
                        
                        if data.get('type') == 'log':
                            log_messages.append(data['data']['message'])
                        elif data.get('type') == 'complete':
                            completed = True
                            break
        
        # Verify we received logs
        assert len(log_messages) > 0
        assert completed
        
        # Check for expected log content
        log_text = ' '.join(log_messages)
        assert 'clean_cache' in log_text.lower() or 'cache' in log_text.lower()
        
        print(f"✓ Received {len(log_messages)} log messages via SSE")
    
    def test_execution_status(self):
        """Test querying execution status."""
        # Start an action
        response = requests.post(
            f"{API_BASE_URL}/api/actions/clean_cache/execute",
            json={"parameters": {}}
        )
        
        execution_id = response.json()["execution_id"]
        
        # Query status immediately
        status = self._get_status(execution_id)
        assert "execution_id" in status
        assert "state" in status
        assert status["execution_id"] == execution_id
        
        # Status should be PENDING or STARTED
        assert status["state"] in ["PENDING", "STARTED", "SUCCESS"]
        
        print(f"✓ Status query successful: {status['state']}")
    
    def test_nonexistent_action(self):
        """Test executing a non-existent action returns 404."""
        response = requests.post(
            f"{API_BASE_URL}/api/actions/nonexistent_action/execute",
            json={"parameters": {}}
        )
        
        assert response.status_code == 404
        print(f"✓ Non-existent action properly rejected")
    
    def test_nested_action_execution(self):
        """Test executing an action that calls other actions (nested)."""
        response = requests.post(
            f"{API_BASE_URL}/api/actions/full_deploy/execute",
            json={"parameters": {"environment": "staging"}}
        )
        
        assert response.status_code == 200
        execution_id = response.json()["execution_id"]
        
        print(f"✓ Nested action submitted: {execution_id}")
        
        # Wait for completion (nested actions take longer)
        self._wait_for_completion(execution_id, timeout=90)
        
        # Verify success
        status = self._get_status(execution_id)
        assert status["state"] == "SUCCESS"
        
        print(f"✓ Nested action completed successfully")
    
    # Helper methods
    def _get_status(self, execution_id: str) -> Dict[str, Any]:
        """Get execution status."""
        response = requests.get(f"{API_BASE_URL}/api/executions/{execution_id}/status")
        assert response.status_code == 200
        return response.json()
    
    def _wait_for_completion(self, execution_id: str, timeout: int = 30):
        """Wait for an execution to complete."""
        start_time = time.time()
        
        while time.time() - start_time < timeout:
            status = self._get_status(execution_id)
            
            if status["ready"]:
                return
            
            time.sleep(0.5)
        
        pytest.fail(f"Execution {execution_id} did not complete within {timeout}s")


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
